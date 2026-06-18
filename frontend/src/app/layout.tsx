import type { Metadata } from 'next';
import './globals.css';

export const metadata: Metadata = {
  title: 'Docker Service Monitor',
  description: 'Real-time service health dashboard for Docker networks',
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="zh-CN">
      <body className="min-h-screen flex flex-col">{children}</body>
    </html>
  );
}
