package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

// ---------------------------------------------------------------------------
//  HTTP prober — tries health-check endpoints in order and returns a result.
// ---------------------------------------------------------------------------

type Prober struct {
	client  *http.Client
	port    int
	timeout time.Duration
}

func NewProber(port int, timeout time.Duration) *Prober {
	return &Prober{
		port:    port,
		timeout: timeout,
		client: &http.Client{
			Timeout: timeout,
			Transport: &http.Transport{
				Dial: (&net.Dialer{Timeout: timeout}).Dial,
			},
			// Don't follow redirects — we want the direct status code
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Probe tries each endpoint in order on the given host.
// Endpoints (tried sequentially):
//   1. GET /v1/models  (OpenAI-compatible)
//   2. GET /health
//   3. GET /
//
// On the first 2xx–4xx response it stops and returns success.
// If all fail, returns the last error / non-success result.
func (p *Prober) Probe(host string) ProbeResult {
	endpoints := []string{
		fmt.Sprintf("/v1/models"),
		fmt.Sprintf("/health"),
		fmt.Sprintf("/"),
	}

	var lastResult ProbeResult

	for _, ep := range endpoints {
		url := fmt.Sprintf("http://%s:%d%s", host, p.port, ep)
		result := p.doGet(url)

		// Accept 2xx, 3xx, 4xx as "alive"
		if result.StatusCode >= 200 && result.StatusCode < 500 {
			return result
		}

		lastResult = result
	}

	return lastResult
}

// ProbeSSE checks if the service supports SSE on the Chat Completions endpoint.
func (p *Prober) ProbeSSE(host string) bool {
	// Try GET first
	url := fmt.Sprintf("http://%s:%d/v1/chat/completions?stream=true", host, p.port)
	if p.isSSE(url) {
		return true
	}

	// Try POST
	url2 := fmt.Sprintf("http://%s:%d/v1/chat/completions", host, p.port)
	return p.isSSE(url2)
}

func (p *Prober) isSSE(url string) bool {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")

	// Use a shorter timeout for SSE probe
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		return false
	}
	// Drain body but don't wait for it all
	io.CopyN(io.Discard, resp.Body, 1024)

	return resp.StatusCode == 200 &&
		(ct == "text/event-stream" || ct == "text/event-stream; charset=utf-8")
}

func (p *Prober) doGet(url string) ProbeResult {
	start := time.Now()
	result := ProbeResult{}

	resp, err := p.client.Get(url)
	if err != nil {
		result.Duration = time.Since(start)
		result.Online = false
		result.StatusCode = 0
		return result
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, 4096)
	bodyBytes, _ := io.ReadAll(limited)

	result.StatusCode = resp.StatusCode
	result.Duration = time.Since(start)
	result.Online = resp.StatusCode >= 200 && resp.StatusCode < 500
	result.Body = string(bodyBytes)

	// Try to parse active_connections from the response
	if result.Body != "" {
		var payload struct {
			ActiveConnections *int `json:"active_connections"`
		}
		if err := json.Unmarshal(bodyBytes, &payload); err == nil && payload.ActiveConnections != nil {
			result.ReportedConn = payload.ActiveConnections
		}
	}

	return result
}
