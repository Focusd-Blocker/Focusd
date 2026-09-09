package main

import (
	"context"
	_ "embed"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"focusd/internal/blocklist"
	"focusd/internal/extension"
	"focusd/internal/stats"
	"focusd/internal/updater"
)

//go:embed VERSION
var versionFile string

var daemonVersion = "dev"

func init() {
	if daemonVersion == "dev" {
		daemonVersion = strings.TrimSpace(versionFile)
	}
}

func main() {
	log.Println("Starting Focusd daemon", daemonVersion)
	if len(os.Args) > 1 && os.Args[1] == "--apply-update" {
		if err := updater.ApplyPendingUpdate(os.Args[2:]); err != nil {
			log.Fatal("update: applying update:", err)
		}
		return
	}
	if len(os.Args) > 2 && os.Args[1] == "--cleanup-updater" {
		_ = os.Remove(os.Args[2])
		os.Args = os.Args[2:]
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := stats.Init(daemonVersion); err != nil {
		log.Fatal("failed to init stats:", err)
	}

	log.Println("Starting list updating loop")
	blocklist.StartUpdateLoop(ctx)

	log.Println("Starting extension auto-update loop")
	go extension.StartUpdateLoop(ctx)
	log.Println("Starting daemon auto-update loop")
	go updater.StartUpdateLoop(ctx, daemonVersion)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", stats.HandleDashboard)
	mux.HandleFunc("GET /blocklist", blocklist.HandleBlocklist)
	mux.HandleFunc("GET /blocklist/count", blocklist.HandleBlocklistCount)
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
