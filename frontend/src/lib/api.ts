// ---------------------------------------------------------------------------
//  Types
// ---------------------------------------------------------------------------

export interface ServiceSnapshot {
  id: string;
  name: string;
  ipv4: string;
  image: string;
  online: boolean;
  statusCode: number;
  responseTime: number;
  uptimeMs: number;
  uptimeHuman: string;
  errorRate: number;
  activeConns: number;
  reportedConns: number | null;
  sseCapable: boolean;
  cpuPercent: number;
  memoryUsage: string;
  firstSeen: number;
  lastOnline: number | null;
  lastChecked: number;
  lastOffline: number | null;
  consecutiveFailures: number;
}

export interface StatsResponse {
  timestamp: number;
  total: number;
  online: number;
  offline: number;
  avgResponseTimeMs: number;
}

// ---------------------------------------------------------------------------
//  API helpers
// ---------------------------------------------------------------------------

const BASE = '';

export async function fetchServices(): Promise<ServiceSnapshot[]> {
  const res = await fetch(`${BASE}/api/services`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const data = await res.json();
  return data.services as ServiceSnapshot[];
}

export async function fetchStats(): Promise<StatsResponse> {
  const res = await fetch(`${BASE}/api/stats`);
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  return res.json();
}

// ---------------------------------------------------------------------------
//  SSE client
// ---------------------------------------------------------------------------

export type ConnectionStatus = 'connecting' | 'connected' | 'error';

export function connectSSE(
  onMetrics: (data: ServiceSnapshot[]) => void,
  onStatusChange: (status: ConnectionStatus) => void,
): EventSource {
  const es = new EventSource(`${BASE}/api/events`);

  es.addEventListener('metrics', (e: MessageEvent) => {
    try {
      const data = JSON.parse(e.data) as ServiceSnapshot[];
      onMetrics(data);
    } catch (err) {
      console.error('SSE parse error:', err);
    }
  });

  es.addEventListener('open', () => {
    onStatusChange('connected');
  });

  es.addEventListener('error', () => {
    onStatusChange('error');
  });

  onStatusChange('connecting');

  return es;
}
