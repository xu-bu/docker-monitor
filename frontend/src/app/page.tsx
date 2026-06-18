'use client';

import { useEffect, useState, useCallback } from 'react';
import Header from '@/components/Header';
import StatusBar from '@/components/StatusBar';
import ServiceGrid from '@/components/ServiceGrid';
import { fetchServices, connectSSE } from '@/lib/api';
import type { ServiceSnapshot, ConnectionStatus } from '@/lib/api';

export default function DashboardPage() {
  const [services, setServices] = useState<ServiceSnapshot[]>([]);
  const [connStatus, setConnStatus] = useState<ConnectionStatus>('connecting');
  const [lastUpdated, setLastUpdated] = useState<number | null>(null);

  const onMetrics = useCallback((data: ServiceSnapshot[]) => {
    setServices(data);
    setLastUpdated(Date.now());
  }, []);

  const onStatusChange = useCallback((s: ConnectionStatus) => {
    setConnStatus(s);
    // If we got disconnected, try reconnecting the whole page
    if (s === 'error') {
      setTimeout(() => window.location.reload(), 5000);
    }
  }, []);

  useEffect(() => {
    // Initial fetch
    fetchServices()
      .then(onMetrics)
      .catch(() => {});

    // SSE connection
    const es = connectSSE(onMetrics, onStatusChange);
    return () => es.close();
  }, [onMetrics, onStatusChange]);

  // Aggregate stats from the service list
  const total  = services.length;
  const online = services.filter((s) => s.online).length;
  const avgResp = online > 0
    ? Math.round(services.filter((s) => s.online).reduce((a, s) => a + s.responseTime, 0) / online)
    : null;

  return (
    <>
      <Header
        total={total}
        online={online}
        offline={total - online}
        avgResponseTimeMs={avgResp}
      />
      <main className="flex-1 max-w-7xl w-full mx-auto px-6 pb-8">
        <StatusBar
          status={connStatus}
          serviceCount={total}
          offlineCount={total - online}
          lastUpdated={lastUpdated}
        />
        <ServiceGrid services={services} />
      </main>
      <footer className="bg-slate-900 border-t border-slate-700/50 px-6 py-3 text-xs text-slate-500">
        <div className="max-w-7xl mx-auto flex items-center gap-2">
          <span>Docker Service Monitor v1.0.0</span>
          <span className="opacity-30">·</span>
          <span>Auto-refreshing via SSE</span>
        </div>
      </footer>
    </>
  );
}
