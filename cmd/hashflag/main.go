// Command hashflag runs the Hashflag game server.
package main

import (
	"context"
	"errors"
	"flag"
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
	"hashflag/internal/server"
	"hashflag/internal/stats"
)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	configDir := flag.String("config-dir", "./configs", "directory of game config presets")
	configID := flag.String("config", "standard", "config preset id")
	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data-dir", "./data", "directory for board.json and stats.jsonl")
	webDir := flag.String("web-dir", "./web/dist", "built web client to serve at /")
	var origins listFlag
	flag.Var(&origins, "origin", "extra allowed WebSocket Origin pattern (repeatable)")
	flag.Parse()

	cfg, err := config.Resolve(*configDir, *configID)
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	boardPath := filepath.Join(*dataDir, "board.json")
	board, err := factions.LoadBoard(boardPath)
	if err != nil {
		log.Printf("leaderboard: %v; starting with an empty board", err)
		board = factions.NewBoard(boardPath)
	}
	statsLog := stats.NewLogger(filepath.Join(*dataDir, "stats.jsonl"))

	allowed := append([]string{}, server.DefaultAllowedOrigins...)
	allowed = append(allowed, origins...)
	room := server.NewRoom(cfg, board, statsLog, server.RoomOptions{AllowedOrigins: allowed})

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
