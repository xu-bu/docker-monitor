package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
//  Configuration (from environment variables)
// ---------------------------------------------------------------------------

var (
	monitorPort       = envInt("MONITOR_PORT", 8080)
	targetPort        = envInt("MONITOR_TARGET_PORT", 3000)
	monitorInterval   = envDuration("MONITOR_INTERVAL", 5*time.Second)
	monitorTimeout    = envDuration("MONITOR_TIMEOUT", 3*time.Second)
	discoveryInterval = envDuration("DISCOVERY_INTERVAL", 15*time.Second)
	networkName       = os.Getenv("MONITOR_NETWORK")
)

func main() {
	uptimeSec = float64(time.Now().Unix())

	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("[main] Docker Service Monitor starting")
	log.Printf("[main] probe interval=%s timeout=%s discovery=%s",
		monitorInterval, monitorTimeout, discoveryInterval)

	// ------------------------------------------------------------------
	//  Bootstrap components
	// ------------------------------------------------------------------

	discoverer := NewDiscoverer(networkName, discoveryInterval)
	prober := NewProber(targetPort, monitorTimeout)
	broker := NewSSEBroker()
	monitor := NewMonitor(discoverer, prober, monitorInterval, broker)

	// ------------------------------------------------------------------
	//  Start background workers
	// ------------------------------------------------------------------

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go discoverer.Start(ctx)
	go monitor.Start(ctx)

	// ------------------------------------------------------------------
	//  HTTP mux (Go 1.22+ pattern routing)
	// ------------------------------------------------------------------

	mux := http.NewServeMux()

	server := &Server{
		monitor: monitor,
		broker:  broker,
	}

	// Serve Next.js static export from ./public on disk.
	// In Docker, the multi-stage build copies the Next.js output here.
	// For local dev, create a symlink or copy the frontend/out/ directory.
	server.static = http.FileServer(http.Dir("./public"))
	log.Printf("[main] serving static files from ./public")

	// REST API
	mux.HandleFunc("GET /api/services", server.handleServices)
	mux.HandleFunc("GET /api/services/{id}", server.handleServices)
	mux.HandleFunc("GET /api/stats", server.handleStats)
	mux.HandleFunc("GET /health", server.handleHealth)

	// SSE stream
	mux.HandleFunc("GET /api/events", server.handleSSE)

	// OpenAI-compatible endpoint
	mux.HandleFunc("GET /v1/chat/completions", server.handleChatCompletions)
	mux.HandleFunc("POST /v1/chat/completions", server.handleChatCompletions)

	// Next.js static export (catch-all)
	mux.HandleFunc("/", server.handleIndex)

	// Wrap with CORS
	handler := corsMiddleware(mux)

	// ------------------------------------------------------------------
	//  HTTP server
	// ------------------------------------------------------------------

	addr := ":" + strconv.Itoa(monitorPort)
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // required for SSE (streaming)
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Println("[main] shutting down...")
		cancel()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		httpServer.Shutdown(shutdownCtx)
	}()

	log.Printf("[main] listening on %s", addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[main] server error: %v", err)
	}
	log.Println("[main] stopped")
}

// ---------------------------------------------------------------------------
//  Env helpers
// ---------------------------------------------------------------------------

func envInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return defaultVal
}

func envDuration(key string, defaultVal time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if ms, err := strconv.Atoi(v); err == nil {
			return time.Duration(ms) * time.Millisecond
		}
	}
	return defaultVal
}
