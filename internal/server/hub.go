package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	outBuffer     = 64              // queued outbound frames before a client is dropped
	minRate       = 20              // floor of the per-connection message limit, per second
	discardLimit  = 64 << 10        // oversized frames above this close the connection
	writeTimeout  = 5 * time.Second // per-frame write deadline
	closeDeadline = 3 * time.Second // graceful close budget on shutdown
)

// rateLimitFor is the per-connection message limit (messages per second,
// also the burst size) for a tick length. A client may send one intent per
// state frame, so the limit scales with the tick rate: at least minRate, and
// three times the number of ticks per second.
func rateLimitFor(tickMs int) float64 {
	if tickMs <= 0 {
		return minRate
	}
	return math.Max(minRate, math.Ceil(3000/float64(tickMs)))
}

// conn is one WebSocket client. Its reader goroutine feeds the room inbox;
// its writer goroutine drains out. Only the room goroutine sends on out and
// reads or writes the room-owned fields.
type conn struct {
	ws      *websocket.Conn
	out     chan []byte
	quit    chan struct{} // closed by kill; stops the writer
	closing atomic.Bool
	greeted atomic.Bool // a valid hello has been accepted

	// Room-owned.
	sess *session
	dead bool
}

// kill closes the connection immediately. Safe to call more than once and
// from any goroutine.
func (c *conn) kill(code websocket.StatusCode, reason string) {
	if !c.closing.CompareAndSwap(false, true) {
		return
	}
	close(c.quit)
	if code == 0 {
		c.ws.CloseNow()
		return
	}
	// Close sends a close frame and waits briefly for the peer's reply.
	go c.ws.Close(code, reason)
}

// closeGracefully is kill with a status code, waiting for the handshake.
func (c *conn) closeGracefully(code websocket.StatusCode, reason string) {
	if !c.closing.CompareAndSwap(false, true) {
		return
	}
	close(c.quit)
	c.ws.Close(code, reason)
}

// NewHub returns the HTTP handler serving the WebSocket endpoint at /ws.
func NewHub(r *Room) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", func(w http.ResponseWriter, req *http.Request) {
		ip := remoteIP(req.RemoteAddr)
		if status := r.acquireConn(ip); status != 0 {
			http.Error(w, http.StatusText(status), status)
			return
		}
		defer r.releaseConn(ip)
		ws, err := websocket.Accept(w, req, &websocket.AcceptOptions{
			OriginPatterns: r.opts.AllowedOrigins,
		})
		if err != nil {
			return // Accept already wrote the HTTP error
		}
		serveConn(r, ws)
	})
	return mux
}

// remoteIP is the host part of a RemoteAddr. Proxy headers are not trusted.
func remoteIP(addr string) string {
	if host, _, err := net.SplitHostPort(addr); err == nil {
		return host
	}
	return addr
}

// acquireConn reserves a connection slot for ip. It returns 0 on success,
// or the HTTP status to reject with: 503 when the room is full, 429 when ip
// has too many connections. Every success must be paired with releaseConn.
func (r *Room) acquireConn(ip string) int {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	if r.nConns >= r.opts.MaxConns {
		return http.StatusServiceUnavailable
	}
	if r.ipConns[ip] >= r.opts.MaxConnsPerIP {
		return http.StatusTooManyRequests
	}
	r.nConns++
	r.ipConns[ip]++
	return 0
}

func (r *Room) releaseConn(ip string) {
	r.connMu.Lock()
	defer r.connMu.Unlock()
	r.nConns--
	if r.ipConns[ip]--; r.ipConns[ip] <= 0 {
		delete(r.ipConns, ip)
	}
}

// serveConn registers the connection with the room, then runs the reader
// until the connection ends.
func serveConn(r *Room, ws *websocket.Conn) {
	ws.SetReadLimit(-1) // size is enforced below so oversized frames don't close
	c := &conn{ws: ws, out: make(chan []byte, outBuffer), quit: make(chan struct{})}
	if !r.post(roomMsg{open: c}) {
		c.closeGracefully(websocket.StatusGoingAway, "server shutting down")
		return
	}
	go c.writeLoop()
	hello := time.AfterFunc(r.opts.HelloTimeout, func() {
		if !c.greeted.Load() {
			c.kill(websocket.StatusPolicyViolation, "hello timeout")
		}
	})
	defer hello.Stop()
	go c.keepAlive(r.opts.PingInterval, r.opts.PingTimeout)
	c.readLoop(r)
	r.post(roomMsg{closed: c})
	c.kill(0, "") // no-op if a graceful close is already under way
}

// keepAlive pings the client every interval and closes the connection when a
// ping goes unanswered for timeout. It stops when the connection is killed.
func (c *conn) keepAlive(interval, timeout time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-c.quit:
			return
		case <-t.C:
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			err := c.ws.Ping(ctx)
			cancel()
			if err != nil {
				c.kill(0, "")
				return
			}
		}
	}
}

func (c *conn) writeLoop() {
	for {
		select {
		case <-c.quit:
			return
		case b := <-c.out:
			ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
			err := c.ws.Write(ctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				c.kill(0, "")
				return
			}
		}
	}
}

// readLoop parses frames and forwards them to the room. Oversized, invalid
// or unknown frames are dropped; exceeding the rate limit closes the
// connection with StatusPolicyViolation.
func (c *conn) readLoop(r *Room) error {
	rate := rateLimitFor(r.cfg.TickMs)
	tokens := rate
	last := time.Now()
	buf := make([]byte, MaxFrameBytes+1)
	for {
		_, rd, err := c.ws.Reader(context.Background())
		if err != nil {
			return err
		}
		n, err := io.ReadFull(rd, buf)
		if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
			return err
		}
		oversized := n > MaxFrameBytes
		if oversized {
			m, err := io.Copy(io.Discard, io.LimitReader(rd, discardLimit))
			if err != nil {
				return err
			}
			if m == discardLimit {
				c.kill(websocket.StatusMessageTooBig, "frame too large")
				return errors.New("frame too large")
			}
		}

		now := time.Now()
		tokens = min(rate, tokens+now.Sub(last).Seconds()*rate)
		last = now
		if tokens < 1 {
			c.kill(websocket.StatusPolicyViolation, "rate limit exceeded")
			return errors.New("rate limit exceeded")
		}
		tokens--

		if oversized {
			continue
		}
		var m ClientMsg
		if json.Unmarshal(buf[:n], &m) != nil || !validClientType(m.T) {
			continue
		}
		if !r.post(roomMsg{from: c, msg: m}) {
			return errors.New("room closed")
		}
	}
}

// post delivers m to the room, returning false if the room has stopped.
func (r *Room) post(m roomMsg) bool {
	select {
	case r.inbox <- m:
		return true
	case <-r.done:
		return false
	}
}

// send queues b for c without blocking. A client whose buffer is full is
// dropped so the tick loop never waits on a slow reader. Room goroutine only.
func (r *Room) send(c *conn, b []byte) {
	if c.dead {
		return
	}
	select {
	case c.out <- b:
	default:
		log.Printf("server: dropping slow client")
		r.dropConn(c, websocket.StatusTryAgainLater, "too slow")
	}
}

func (r *Room) sendJSON(c *conn, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("server: marshal: %v", err)
		return
	}
	r.send(c, b)
}
