'use client';

interface HeaderProps {
  total: number;
  online: number;
  offline: number;
  avgResponseTimeMs: number | null;
}

export default function Header({ total, online, offline, avgResponseTimeMs }: HeaderProps) {
  return (
    <header className="sticky top-0 z-50 bg-slate-900/80 backdrop-blur-md border-b border-slate-700/50 px-6 py-3">
      <div className="max-w-7xl mx-auto flex items-center justify-between flex-wrap gap-3">
        {/* Brand */}
        <div className="flex items-center gap-3">
          <svg className="w-9 h-9 text-blue-500 flex-shrink-0" viewBox="0 0 24 24" fill="none"
               stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="10" />
            <path d="M2 12h20M12 2a15.3 15.3 0 0 1 4 10 15.3 15.3 0 0 1-4 10 15.3 15.3 0 0 1-4-10 15.3 15.3 0 0 1 4-10z" />
          </svg>
          <div>
            <h1 className="text-lg font-bold tracking-tight text-slate-100">Service Monitor</h1>
            <p className="text-xs text-slate-400">Docker Network Health Dashboard</p>
          </div>
        </div>

        {/* Stats */}
        <div className="flex gap-2">
          <StatCard label="Total" value={String(total)} className="text-slate-100" />
          <StatCard label="Online" value={String(online)} className="text-green-400" />
          <StatCard label="Offline" value={String(offline)} className="text-red-400" />
          <StatCard
            label="Avg ms"
            value={avgResponseTimeMs !== null ? `${avgResponseTimeMs}ms` : '—'}
            className="text-blue-400"
          />
        </div>
      </div>
    </header>
  );
}

function StatCard({ label, value, className }: { label: string; value: string; className: string }) {
  return (
    <div className="bg-slate-800 border border-slate-700 rounded-lg px-3 py-1.5 text-center min-w-[64px]">
      <span className={`block text-lg font-bold tabular-nums leading-tight ${className}`}>
        {value}
      </span>
      <span className="block text-[0.65rem] uppercase tracking-wider text-slate-500">
        {label}
      </span>
    </div>
  );
}
