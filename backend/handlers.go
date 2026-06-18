package main

import (
	"encoding/json"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
//  Server — holds references to the monitor and broker.
// ---------------------------------------------------------------------------

type Server struct {
	monitor *Monitor
	broker  *SSEBroker
	static  http.Handler // file server for embedded Next.js static files
}

// ---------------------------------------------------------------------------
//  Handlers
// ---------------------------------------------------------------------------

// handleServices returns the full snapshot of all services.
func (s *Server) handleServices(w http.ResponseWriter, r *http.Request) {
	services := s.monitor.Snapshot()

	// Support single-service lookup: /api/services/{id}
	id := r.PathValue("id")
	if id != "" {
		svc := s.monitor.GetService(id)
		if svc == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
			return
		}
		writeJSON(w, http.StatusOK, svc)
		return
	}

	resp := map[string]interface{}{
		"timestamp": time.Now().UnixMilli(),
		"count":     len(services),
		"services":  services,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleStats returns aggregate statistics.
func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	services := s.monitor.Snapshot()
	total := len(services)
	online := 0
	var sumResp int64
	for _, svc := range services {
		if svc.Online {
			online++
			sumResp += svc.ResponseTimeMs
		}
	}

	avgResp := float64(0)
	if online > 0 {
		avgResp = float64(sumResp) / float64(online)
	}

	resp := map[string]interface{}{
		"timestamp":        time.Now().UnixMilli(),
		"total":            total,
		"online":           online,
		"offline":          total - online,
		"avgResponseTimeMs": avgResp,
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleHealth returns the monitor's own health status.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	services := s.monitor.Snapshot()
	allUp := true
	for _, svc := range services {
		if !svc.Online {
			allUp = false
			break
		}
	}

	statusCode := http.StatusOK
	status := "healthy"
	if !allUp && len(services) > 0 {
		statusCode = http.StatusServiceUnavailable
		status = "degraded"
	}

	onlineCount := 0
	for _, s := range services {
		if s.Online {
			onlineCount++
		}
	}
	resp := map[string]interface{}{
		"status":   status,
		"uptime":   uptimeSec,
		"services": len(services),
		"online":   onlineCount,
	}
	writeJSON(w, statusCode, resp)
}

// handleSSE proxies to the SSE broker's ServeHTTP.
func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	s.broker.ServeHTTP(w, r)
}

// handleIndex serves the Next.js static export.
//
// IMPORTANT: Do not rewrite "/" → "/index.html" here.
// http.FileServer already serves index.html for directory paths,
// and rewriting causes a redirect loop because FileServer has
// built-in behavior that 301-redirects any path ending in
// "/index.html" back to the directory root ("/").
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	s.static.ServeHTTP(w, r)
}

// ---------------------------------------------------------------------------
//  Middleware
// ---------------------------------------------------------------------------

// corsMiddleware adds permissive CORS headers for development.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

var uptimeSec float64 // set once at startup

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
