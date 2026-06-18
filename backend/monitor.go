package main

import (
	"context"
	"sync"
	"time"
)

// ---------------------------------------------------------------------------
//  Monitor — periodically probes discovered containers and emits snapshots.
// ---------------------------------------------------------------------------

type Monitor struct {
	discoverer *Discoverer
	prober     *Prober
	interval   time.Duration

	mu       sync.RWMutex
	services map[string]*ServiceState // container ID → state

	// SSE broadcasting
	broker  *SSEBroker
}

func NewMonitor(discoverer *Discoverer, prober *Prober, interval time.Duration, broker *SSEBroker) *Monitor {
	return &Monitor{
		discoverer: discoverer,
		prober:     prober,
		interval:   interval,
		services:   make(map[string]*ServiceState),
		broker:     broker,
	}
}

// Start begins the periodic monitoring loop (blocking — run in a goroutine).
func (m *Monitor) Start(ctx context.Context) {
	// Do an immediate first tick
	m.tick()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.tick()
		case <-ctx.Done():
			return
		}
	}
}

// Snapshot returns a JSON-safe copy of all service states.
func (m *Monitor) Snapshot() []ServiceState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]ServiceState, 0, len(m.services))
	for _, s := range m.services {
		out = append(out, s.Snapshot())
	}
	return out
}

// GetService returns a snapshot for a single container, or nil.
func (m *Monitor) GetService(id string) *ServiceState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	s, ok := m.services[id]
	if !ok {
		return nil
	}
	snap := s.Snapshot()
	return &snap
}

// ---------------------------------------------------------------------------
//  Tick
// ---------------------------------------------------------------------------

func (m *Monitor) tick() {
	containers := m.discoverer.List()
	if len(containers) == 0 {
		return
	}

	// Prune stale service states
	m.mu.Lock()
	valid := make(map[string]bool, len(containers))
	for _, c := range containers {
		valid[c.ID] = true
	}
	for id := range m.services {
		if !valid[id] {
			delete(m.services, id)
		}
	}
	m.mu.Unlock()

	// Fan-out probes concurrently
	var wg sync.WaitGroup
	for _, c := range containers {
		wg.Add(1)
		go func(ci ContainerInfo) {
			defer wg.Done()
			m.probeOne(ci)
		}(c)
	}
	wg.Wait()

	// Broadcast to SSE clients
	snapshot := m.Snapshot()
	m.broker.Broadcast(snapshot)
}

func (m *Monitor) probeOne(c ContainerInfo) {
	host := c.IPv4
	if host == "" {
		return
	}

	result := m.prober.Probe(host)

	// Get or create service state
	m.mu.Lock()
	svc, ok := m.services[c.ID]
	if !ok {
		svc = NewServiceState(c)
		m.services[c.ID] = svc
	}
	m.mu.Unlock()

	svc.RecordProbe(result)

	// SSE capability check every 30 probes
	if result.Online {
		svc.mu.Lock()
		count := svc.probeCount
		svc.mu.Unlock()
		if count%30 == 0 {
			sseOK := m.prober.ProbeSSE(host)
			svc.SetSSECapable(sseOK)
		}
	}
}

// SetBroker updates the SSE broker reference (used during init).
func (m *Monitor) SetBroker(b *SSEBroker) {
	m.broker = b
}
