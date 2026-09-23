import { Component, Suspense, useEffect, type ReactNode } from 'react';
import { Link, Navigate, Route, Routes, useParams } from 'react-router';
import { QueryProvider } from '~/lib/query-client';
import { pages, redirects, type Page } from './routes';
import AgentProvider from '~/components/agent/AgentProvider';

function PageView({ page }: { page: Page }) {
  const params = useParams();
  const title = typeof page.title === 'function' ? page.title(params) : page.title;
  useEffect(() => { document.title = `${title} - Threadify`; }, [title]);
  const Content = page.component;
  return <Content />;
}

function PricingRedirect() {
  useEffect(() => { window.location.replace('https://threadify.dev/pricing'); }, []);
  return <a href="https://threadify.dev/pricing">View Threadify pricing</a>;
}

class AppErrorBoundary extends Component<{ children: ReactNode }, { failed: boolean }> {
  state = { failed: false };
  static getDerivedStateFromError() { return { failed: true }; }
  render() {
    if (this.state.failed) return <div role="alert" className="p-8 text-sm">Unable to load this page. <a href={window.location.href} className="underline">Reload</a></div>;
    return this.props.children;
  }
}

export default function App() {
  return (
    <AppErrorBoundary>
      <QueryProvider>
        <AgentProvider>
          <Suspense fallback={<div role="status" className="p-8 text-sm text-gray-500">Loading Threadify…</div>}>
            <Routes>
              {pages.map(page => <Route key={page.path} path={page.path} element={<PageView page={page} />} />)}
              {Object.entries(redirects).map(([path, to]) => <Route key={path} path={path} element={<Navigate to={to} replace />} />)}
              <Route path="/pricing" element={<PricingRedirect />} />
              <Route path="*" element={<div className="p-8 text-sm">Page not found. <Link to="/u/dashboard" className="underline">Open dashboard</Link></div>} />
            </Routes>
          </Suspense>
        </AgentProvider>
      </QueryProvider>
    </AppErrorBoundary>
  );
}
