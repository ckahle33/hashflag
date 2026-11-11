// Command hashflag runs the Hashflag game server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hashflag/internal/config"
	"hashflag/internal/factions"
	"hashflag/internal/game"
	"hashflag/internal/server"
	"hashflag/internal/stats"
)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

// checkRunnable reports whether cfg can start a round. Validate checks fields
// one by one; only generating a map shows whether the geometry fits together.
func checkRunnable(cfg *config.Config) error {
	if _, err := game.NewWorld(cfg, 1); err != nil {
		return fmt.Errorf("config %q cannot generate a map: %w", cfg.ID, err)
	}
	return nil
}

// openBoard loads the leaderboard at path. If it cannot be read it returns a
// usable empty board stored at path+".recovered" together with the error, so
// the original file is never overwritten.
func openBoard(path string) (*factions.Board, error) {
	b, err := factions.LoadBoard(path)
	if err != nil {
		return factions.NewBoard(path + ".recovered"), err
	}
	return b, nil
}

func main() {
	configDir := flag.String("config-dir", "./configs", "directory of game config presets")
	configID := flag.String("config", "standard", "config preset id")
	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data-dir", "./data", "directory for board.json and stats.jsonl")
	webDir := flag.String("web-dir", "./web/dist", "built web client to serve at /")
	var origins listFlag
	maxConns := flag.Int("max-conns", 0, "maximum open WebSockets (0 = 4 x maxPerTeam, at least 50)")
	maxPerIP := flag.Int("max-conns-per-ip", 0, "maximum open WebSockets per client IP (0 = 20)")
	flag.Var(&origins, "origin", "extra allowed WebSocket Origin pattern (repeatable)")
	flag.Parse()

	cfg, err := config.Resolve(*configDir, *configID)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := checkRunnable(cfg); err != nil {
		log.Fatalf("config: %v", err)
	}

	boardPath := filepath.Join(*dataDir, "board.json")
	board, err := openBoard(boardPath)
	if err != nil {
		log.Printf("leaderboard: %v; leaving %s untouched and starting an empty board at %s.recovered", err, boardPath, boardPath)
	}
	statsLog := stats.NewLogger(filepath.Join(*dataDir, "stats.jsonl"))

	allowed := append([]string{}, server.DefaultAllowedOrigins...)
	allowed = append(allowed, origins...)
	room := server.NewRoom(cfg, board, statsLog, server.RoomOptions{AllowedOrigins: allowed, MaxConns: *maxConns, MaxConnsPerIP: *maxPerIP})

	mux := http.NewServeMux()
	mux.Handle("/ws", server.NewHub(room))
	if st, err := os.Stat(*webDir); err == nil && st.IsDir() {
		mux.Handle("/", http.FileServer(http.Dir(*webDir)))
	} else {
		log.Printf("web dir %s not found; serving only /ws", *webDir)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	roomDone := make(chan struct{})
	go func() { room.Run(ctx); close(roomDone) }()

	srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		log.Printf("hashflag %q listening on %s", cfg.ID, *addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("http: %v", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Printf("shutting down")
	<-roomDone // closes every WebSocket and saves the board
	shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
}
