'use client';

import type { ConnectionStatus } from '@/lib/api';

interface StatusBarProps {
  status: ConnectionStatus;
  serviceCount: number;
  offlineCount: number;
  lastUpdated: number | null;
}

export default function StatusBar({ status, serviceCount, offlineCount, lastUpdated }: StatusBarProps) {
  const dotClass =
    status === 'connected'
      ? 'status-dot status-dot--online'
      : status === 'error'
        ? 'status-dot status-dot--offline'
        : 'status-dot status-dot--connecting';

  const statusText =
    status === 'connected'
      ? serviceCount === 0
        ? 'No services discovered'
        : offlineCount === 0
          ? `All ${serviceCount} service(s) healthy`
          : `${offlineCount}/${serviceCount} service(s) offline`
      : status === 'error'
        ? 'Disconnected — retrying…'
        : 'Connecting…';

  return (
    <div className="max-w-7xl mx-auto mt-3 px-6 flex items-center gap-2 text-sm text-slate-400">
      <span className={dotClass} />
      <span>{statusText}</span>

      {lastUpdated && (
        <span className="ml-auto font-mono text-[0.7rem] text-slate-500">
          {new Date(lastUpdated).toLocaleTimeString()}
        </span>
      )}
    </div>
  );
}
