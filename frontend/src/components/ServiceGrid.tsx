'use client';

import type { ServiceSnapshot } from '@/lib/api';
import ServiceCard from './ServiceCard';

interface ServiceGridProps {
  services: ServiceSnapshot[];
}

export default function ServiceGrid({ services }: ServiceGridProps) {
  if (services.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-slate-500">
        <svg className="w-12 h-12 mb-4 opacity-30" viewBox="0 0 24 24" fill="none"
             stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round">
          <rect x="2" y="3" width="20" height="14" rx="2" />
          <path d="M8 21h8M12 17v4" />
        </svg>
        <h3 className="text-base font-medium text-slate-400 mb-1">No services discovered</h3>
        <p className="text-sm">Waiting for containers to appear on the network…</p>
      </div>
    );
  }

  return (
    <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4 mt-4">
      {services.map((svc) => (
        <ServiceCard key={svc.id} service={svc} />
      ))}
    </div>
  );
}
