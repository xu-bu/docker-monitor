'use client';

import type { ServiceSnapshot } from '@/lib/api';

interface ServiceCardProps {
  service: ServiceSnapshot;
}

export default function ServiceCard({ service }: ServiceCardProps) {
  const isOnline = service.online;
  const statusClass = isOnline ? 'border-l-green-500' : 'border-l-red-500';

  return (
    <div
      className={`bg-slate-800/80 border border-slate-700/60 border-l-[3px] rounded-xl px-4 pt-3.5 pb-3
                  transition-all duration-200 hover:bg-slate-800 hover:border-slate-600 hover:shadow-xl
                  ${statusClass}`}
    >
      {/* Header */}
      <div className="flex items-center gap-2.5 mb-3">
        <span className={`status-dot ${isOnline ? 'status-dot--online' : 'status-dot--offline'}`} />
        <h2 className="text-sm font-semibold text-slate-200 truncate flex-1">{service.name}</h2>
        {service.sseCapable && (
          <span className="text-[0.55rem] font-semibold uppercase tracking-widest px-1.5 py-0.5 rounded
                           bg-blue-500/10 text-blue-400 border border-blue-500/20">
            SSE
          </span>
        )}
      </div>

      {/* Metrics grid */}
      <div className="grid grid-cols-3 gap-x-3 gap-y-2 mb-2.5">
        <Metric label="Response" value={fmtMs(service.responseTime)} className="text-blue-400" />
        <Metric label="Conns" value={String(service.activeConns)} className="text-purple-400" />
        <Metric
          label="Uptime"
          value={isOnline ? service.uptimeHuman : 'OFFLINE'}
          className={isOnline ? 'text-amber-400' : 'text-red-400'}
        />
        <Metric
          label="Error Rate"
          value={fmtPct(service.errorRate)}
          className={service.errorRate > 0.3 ? 'text-red-400' : 'text-slate-400'}
        />
        <Metric
          label="CPU"
          value={service.cpuPercent >= 0 ? `${service.cpuPercent}%` : '—'}
          className="text-cyan-400"
        />
        <Metric
          label="Memory"
          value={service.memoryUsage || '—'}
          className="text-emerald-400"
        />
      </div>

      {/* Meta footer */}
      <div className="border-t border-slate-700/50 pt-2 text-[0.65rem] text-slate-500 flex flex-col gap-0.5">
        {service.image && (
          <span className="truncate">{service.image}</span>
        )}
        <span className="font-mono">
          {service.lastChecked ? `Last: ${fmtRelative(service.lastChecked)}` : ''}
        </span>
      </div>
    </div>
  );
}

// ---- Sub-components & helpers -------------------------------------------

function Metric({ label, value, className }: { label: string; value: string; className: string }) {
  return (
    <div className="flex flex-col">
      <span className={`text-base font-bold tabular-nums font-mono leading-tight ${className}`}>
        {value}
      </span>
      <span className="text-[0.6rem] uppercase tracking-widest text-slate-500">{label}</span>
    </div>
  );
}

function fmtMs(ms: number): string {
  if (ms <= 0) return '—';
  if (ms < 1000) return `${Math.round(ms)}ms`;
  return `${(ms / 1000).toFixed(1)}s`;
}

function fmtPct(rate: number): string {
  if (rate == null) return '—';
  return `${(rate * 100).toFixed(0)}%`;
}

function fmtRelative(ts: number): string {
  const diff = Date.now() - ts;
  if (diff < 5000) return 'just now';
  if (diff < 60000) return `${Math.round(diff / 1000)}s ago`;
  if (diff < 3600000) return `${Math.round(diff / 60000)}m ago`;
  return `${Math.round(diff / 3600000)}h ago`;
}
