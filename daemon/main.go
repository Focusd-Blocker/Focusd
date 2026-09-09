package main

import (
	"context"
	"log"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"focusd/internal/blocklist"
	"focusd/internal/extension"
	"focusd/internal/stats"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := stats.Init(); err != nil {
		log.Fatal("failed to init stats:", err)
	}

	log.Println("Starting list updating loop")
	blocklist.StartUpdateLoop(ctx)

	log.Println("Starting extension auto-update loop")
	go extension.StartUpdateLoop(ctx)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", stats.HandleDashboard)
	mux.HandleFunc("GET /blocklist", blocklist.HandleBlocklist)
	mux.HandleFunc("GET /stream", blocklist.HandleStream)
	mux.HandleFunc("GET /custom_blocklist", blocklist.HandleCustomBlocklist)
	mux.HandleFunc("POST /custom_blocklist", blocklist.HandleCustomBlocklist)
	mux.HandleFunc("DELETE /custom_blocklist", blocklist.HandleCustomBlocklist)
	mux.HandleFunc("POST /event", stats.HandleEvent)
	mux.HandleFunc("POST /heartbeat", stats.HandleHeartbeat)
	mux.HandleFunc("GET /stats", stats.HandleStats)

	srv := &http.Server{
		Addr:    "127.0.0.1:36287",
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		log.Println("shutting down...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	log.Println("Starting focusd on :36287")
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
