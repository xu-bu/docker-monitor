package main

import (
	"fmt"
	"math"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
//  ContainerInfo — returned by the discoverer
// ---------------------------------------------------------------------------

type ContainerInfo struct {
	ID     string
	Name   string
	IPv4   string
	Image  string
	Labels map[string]string
	State  string // Docker container state (running, exited, ...)
}

// ---------------------------------------------------------------------------
//  ProbeResult — returned by a single HTTP health-check
// ---------------------------------------------------------------------------

type ProbeResult struct {
	Online       bool
	StatusCode   int
	Duration     time.Duration
	Body         string // truncated first 4KB
	ReportedConn *int   // parsed from target JSON, if available
}

// ---------------------------------------------------------------------------
//  ServiceState — aggregated metrics for one container
// ---------------------------------------------------------------------------

type ServiceState struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	IPv4   string `json:"ipv4"`
	Image  string `json:"image"`

	Online         bool    `json:"online"`
	StatusCode     int     `json:"statusCode"`
	ResponseTimeMs int64   `json:"responseTime"`
	UptimeMs       int64   `json:"uptimeMs"`
	UptimeHuman    string  `json:"uptimeHuman"`
	ErrorRate      float64 `json:"errorRate"`
	ActiveConns    int     `json:"activeConns"`
	ReportedConns  *int    `json:"reportedConns"`
	SSECapable     bool    `json:"sseCapable"`

	FirstSeen          int64  `json:"firstSeen"`
	LastOnline         *int64 `json:"lastOnline"`
	LastChecked        int64  `json:"lastChecked"`
	LastOffline        *int64 `json:"lastOffline"`
	ConsecutiveFailures int   `json:"consecutiveFailures"`

	mu            sync.RWMutex
	history       []probeRecord
	historySize   int
	windowSize    int
	probeCount    int64
	firstSeen     time.Time
	lastOnline    time.Time
	lastChecked   time.Time
	lastOffline   time.Time
	sinceRecover  time.Time // reset when transitioning offline→online
}

type probeRecord struct {
	online       bool
	responseTime time.Duration
	ts           time.Time
}

const (
	defaultHistorySize = 120
	defaultWindowSize  = 20
)

func NewServiceState(c ContainerInfo) *ServiceState {
	now := time.Now()
	return &ServiceState{
		ID:     c.ID,
		Name:   c.Name,
		IPv4:   c.IPv4,
		Image:  c.Image,

		firstSeen:    now,
		sinceRecover: now,
		historySize:  defaultHistorySize,
		windowSize:   defaultWindowSize,
		history:      make([]probeRecord, 0, defaultHistorySize),
	}
}

// RecordProbe ingests a probe result and updates derived metrics.
func (s *ServiceState) RecordProbe(r ProbeResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	s.probeCount++
	s.lastChecked = now
	s.StatusCode = r.StatusCode
	s.ResponseTimeMs = r.Duration.Milliseconds()

	if r.Online {
		if !s.Online {
			// Recovery — reset uptime base
			s.sinceRecover = now
			s.ConsecutiveFailures = 0
		}
		s.Online = true
		s.lastOnline = now
		s.UptimeMs = now.Sub(s.sinceRecover).Milliseconds()
	} else {
		s.Online = false
		s.ConsecutiveFailures++
		s.lastOffline = now
		s.UptimeMs = 0
	}

	if r.ReportedConn != nil {
		s.ReportedConns = r.ReportedConn
	}

	// Active connections heuristic
	if s.ReportedConns != nil {
		s.ActiveConns = *s.ReportedConns
	} else if r.Online {
		loadFactor := int(math.Min(10, math.Ceil(float64(r.Duration.Milliseconds())/200)))
		if loadFactor < 1 {
			loadFactor = 1
		}
		s.ActiveConns = loadFactor
	} else {
		s.ActiveConns = 0
	}

	// History ring
	s.history = append(s.history, probeRecord{
		online:       r.Online,
		responseTime: r.Duration,
		ts:           now,
	})
	if len(s.history) > s.historySize {
		s.history = s.history[len(s.history)-s.historySize:]
	}

	// Error rate over sliding window
	window := s.history
	if len(window) > s.windowSize {
		window = window[len(window)-s.windowSize:]
	}
	errs := 0
	for _, h := range window {
		if !h.online {
			errs++
		}
	}
	if len(window) > 0 {
		s.ErrorRate = float64(errs) / float64(len(window))
	} else {
		s.ErrorRate = 0
	}

	// Update JSON-friendly fields
	s.FirstSeen = s.firstSeen.UnixMilli()
	s.LastChecked = now.UnixMilli()
	if !s.lastOnline.IsZero() {
		v := s.lastOnline.UnixMilli()
		s.LastOnline = &v
	} else {
		s.LastOnline = nil
	}
	if !s.lastOffline.IsZero() {
		v := s.lastOffline.UnixMilli()
		s.LastOffline = &v
	} else {
		s.LastOffline = nil
	}
	s.UptimeHuman = formatDuration(s.UptimeMs)
}

// SetSSECapable marks the service as SSE-supporting.
func (s *ServiceState) SetSSECapable(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.SSECapable = v
}

// Snapshot returns a JSON-safe copy of the state.
func (s *ServiceState) Snapshot() ServiceState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return ServiceState{
		ID:                 s.ID,
		Name:               s.Name,
		IPv4:               s.IPv4,
		Image:              s.Image,
		Online:             s.Online,
		StatusCode:         s.StatusCode,
		ResponseTimeMs:     s.ResponseTimeMs,
		UptimeMs:           s.UptimeMs,
		UptimeHuman:        s.UptimeHuman,
		ErrorRate:          s.ErrorRate,
		ActiveConns:        s.ActiveConns,
		ReportedConns:      s.ReportedConns,
		SSECapable:         s.SSECapable,
		FirstSeen:          s.FirstSeen,
		LastOnline:         s.LastOnline,
		LastChecked:        s.LastChecked,
		LastOffline:        s.LastOffline,
		ConsecutiveFailures: s.ConsecutiveFailures,
	}
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

func formatDuration(ms int64) string {
	if ms <= 0 {
		return "—"
	}
	sec := ms / 1000
	if sec < 60 {
		return fmt.Sprintf("%ds", sec)
	}
	min := sec / 60
	sec = sec % 60
	if min < 60 {
		return fmt.Sprintf("%dm %ds", min, sec)
	}
	h := min / 60
	min = min % 60
	if h < 24 {
		return fmt.Sprintf("%dh %dm", h, min)
	}
	d := h / 24
	h = h % 24
	return fmt.Sprintf("%dd %dh", d, h)
}
