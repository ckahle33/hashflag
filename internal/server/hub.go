package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
)

const (
	outBuffer     = 64              // queued outbound frames before a client is dropped
	rateBurst     = 20              // token bucket size
	ratePerSec    = 20.0            // token refill per second
	discardLimit  = 64 << 10        // oversized frames above this close the connection
	writeTimeout  = 5 * time.Second // per-frame write deadline
	closeDeadline = 3 * time.Second // graceful close budget on shutdown
)

// conn is one WebSocket client. Its reader goroutine feeds the room inbox;
// its writer goroutine drains out. Only the room goroutine sends on out and
// reads or writes the room-owned fields.
type conn struct {
	ws      *websocket.Conn
	out     chan []byte
	quit    chan struct{} // closed by kill; stops the writer
	closing atomic.Bool

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
	c.readLoop(r)
	r.post(roomMsg{closed: c})
	c.kill(0, "") // no-op if a graceful close is already under way
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
	tokens := float64(rateBurst)
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
		tokens = min(float64(rateBurst), tokens+now.Sub(last).Seconds()*ratePerSec)
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
