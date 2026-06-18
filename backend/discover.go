package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
)

// ---------------------------------------------------------------------------
//  Discoverer — periodically queries the Docker API for containers on the
//  same network as this monitor container.
// ---------------------------------------------------------------------------

type Discoverer struct {
	cli         *client.Client
	networkName string // explicit; auto-detected when empty
	interval    time.Duration
	myID        string
	myNetworkID string

	containers []ContainerInfo // latest snapshot
	onChange   func(added, removed int)
}

func NewDiscoverer(networkName string, interval time.Duration) *Discoverer {
	return &Discoverer{
		networkName: networkName,
		interval:    interval,
	}
}

// Start begins the periodic discovery loop (blocking call — run in a goroutine).
func (d *Discoverer) Start(ctx context.Context) {
	if err := d.initDockerClient(); err != nil {
		log.Printf("[discover] Docker client init error: %v — will retry", err)
	}

	// Do an immediate first scan
	d.scan(ctx)

	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.scan(ctx)
		case <-ctx.Done():
			return
		}
	}
}

// List returns the latest discovered containers (safe copy).
func (d *Discoverer) List() []ContainerInfo {
	out := make([]ContainerInfo, len(d.containers))
	copy(out, d.containers)
	return out
}

func (d *Discoverer) initDockerClient() error {
	cli, err := client.NewClientWithOpts(
		client.FromEnv,
		client.WithAPIVersionNegotiation(),
	)
	if err != nil {
		return err
	}
	d.cli = cli
	return nil
}

// ---------------------------------------------------------------------------
//  Scan logic
// ---------------------------------------------------------------------------

func (d *Discoverer) scan(ctx context.Context) {
	if d.cli == nil {
		if err := d.initDockerClient(); err != nil {
			log.Printf("[discover] cannot init Docker client: %v", err)
			return
		}
	}

	// 1. Resolve our own container ID (once)
	if d.myID == "" {
		if err := d.resolveSelf(ctx); err != nil {
			log.Printf("[discover] cannot identify self: %v", err)
			return
		}
	}

	// 2. Resolve the network we belong to (once)
	if d.myNetworkID == "" {
		if err := d.resolveNetwork(ctx); err != nil {
			log.Printf("[discover] cannot resolve network: %v", err)
			return
		}
	}

	// 3. List containers on our network
	net, err := d.cli.NetworkInspect(ctx, d.myNetworkID, network.InspectOptions{})
	if err != nil {
		log.Printf("[discover] network inspect error: %v", err)
		return
	}

	var fresh []ContainerInfo

	for cID, netRes := range net.Containers {
		if cID == d.myID {
			continue // skip self
		}

		info, err := d.cli.ContainerInspect(ctx, cID)
		if err != nil {
			continue // container may have been removed mid-list
		}

		c := ContainerInfo{
			ID:     cID,
			Name:   strings.TrimPrefix(info.Name, "/"),
			IPv4:   ipv4FromEndpointResource(netRes),
			Image:  info.Config.Image,
			Labels: info.Config.Labels,
			State:  info.State.Status,
		}
		fresh = append(fresh, c)
	}

	// 4. Detect changes
	oldMap := make(map[string]bool, len(d.containers))
	for _, c := range d.containers {
		oldMap[c.ID] = true
	}
	newMap := make(map[string]bool, len(fresh))
	for _, c := range fresh {
		newMap[c.ID] = true
	}

	var added, removed int
	for _, c := range fresh {
		if !oldMap[c.ID] {
			added++
		}
	}
	for _, c := range d.containers {
		if !newMap[c.ID] {
			removed++
		}
	}

	d.containers = fresh

	if (added > 0 || removed > 0) && d.onChange != nil {
		d.onChange(added, removed)
	}

	if added > 0 || removed > 0 {
		log.Printf("[discover] +%d / -%d containers (total %d)", added, removed, len(fresh))
	}
}

// ---------------------------------------------------------------------------
//  Self-identification
// ---------------------------------------------------------------------------

func (d *Discoverer) resolveSelf(ctx context.Context) error {
	hostname, _ := os.Hostname()
	hostname = strings.ToLower(hostname)

	list, err := d.cli.ContainerList(ctx, container.ListOptions{})
	if err != nil {
		return err
	}

	for _, c := range list {
		for _, n := range c.Names {
			name := strings.TrimPrefix(n, "/")
			if name == hostname || c.ID == hostname || strings.HasPrefix(c.ID, hostname) {
				d.myID = c.ID
				return nil
			}
		}
	}

	// Fallback: read cgroup to get full container ID
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if idx := strings.Index(line, "/docker/"); idx >= 0 {
				id := strings.TrimSpace(line[idx+8:])
				if len(id) >= 64 {
					d.myID = id[:64]
					return nil
				}
			}
		}
	}

	return fmt.Errorf("could not identify this container in Docker API")
}

// ---------------------------------------------------------------------------
//  Network resolution
// ---------------------------------------------------------------------------

func (d *Discoverer) resolveNetwork(ctx context.Context) error {
	// If user specified an explicit network name, use it
	if d.networkName != "" {
		net, err := d.cli.NetworkInspect(ctx, d.networkName, network.InspectOptions{})
		if err != nil {
			return fmt.Errorf("network %q not found: %w", d.networkName, err)
		}
		d.myNetworkID = net.ID
		return nil
	}

	// Auto-detect: pick the first user-defined network
	info, err := d.cli.ContainerInspect(ctx, d.myID)
	if err != nil {
		return err
	}

	var preferred string
	for name := range info.NetworkSettings.Networks {
		if name != "bridge" && name != "none" && name != "host" {
			preferred = name
			break
		}
	}
	if preferred == "" {
		// fallback to first available
		for name := range info.NetworkSettings.Networks {
			preferred = name
			break
		}
	}
	if preferred == "" {
		return fmt.Errorf("container is not attached to any network")
	}

	net, err := d.cli.NetworkInspect(ctx, preferred, network.InspectOptions{})
	if err != nil {
		return err
	}
	d.networkName = preferred
	d.myNetworkID = net.ID
	log.Printf("[discover] watching network %q (%s)", preferred, net.ID[:12])
	return nil
}

// ---------------------------------------------------------------------------
//  Helpers
// ---------------------------------------------------------------------------

func ipv4FromEndpointResource(r network.EndpointResource) string {
	// r.IPv4Address is in "10.0.0.2/24" format — strip the mask
	if idx := strings.IndexByte(r.IPv4Address, '/'); idx >= 0 {
		return r.IPv4Address[:idx]
	}
	return r.IPv4Address
}
