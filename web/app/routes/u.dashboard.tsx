import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router';
import { ArrowUpRight, CheckCircle2, Circle, Code2, FileText, GitBranch, Mail, UserRound } from 'lucide-react';
import { api, type User } from '~/lib/api';
import WorkspacePage from '~/components/WorkspacePage';

const steps = [
  { title: 'Create a contract', description: 'Define the steps and rules for a workflow.', href: '/u/contracts', icon: FileText },
  { title: 'Explore threads', description: 'See how workflow runs progress over time.', href: '/u/threads', icon: GitBranch },
  { title: 'Connect your code', description: 'Create credentials and instrument your services.', href: '/u/developer', icon: Code2 },
];

export default function Dashboard() {
  const navigate = useNavigate();
  const [user, setUser] = useState<User | null>(null);

  useEffect(() => {
    if (!api.isAuthenticated()) {
      navigate('/login');
      return;
    }
    const storedUser = api.getStoredUser();
    if (!storedUser) {
      navigate('/login');
      return;
    }
    if (!storedUser.first_instrumentation_done) {
      navigate('/u/getting-started');
      return;
    }
    setUser(storedUser);
  }, [navigate]);

  if (!user) return <div className="flex min-h-screen items-center justify-center bg-[#f8f8f6] text-sm text-stone-500">Loading workspace…</div>;

  const statuses = [
    { label: 'Email', value: user.email_verified ? 'Verified' : 'Not verified', complete: Boolean(user.email_verified), icon: Mail },
    { label: 'Onboarding', value: user.onboarding_completed ? 'Complete' : 'Pending', complete: Boolean(user.onboarding_completed), icon: UserRound },
    { label: 'Instrumentation', value: user.first_instrumentation_done ? 'Connected' : 'Not started', complete: Boolean(user.first_instrumentation_done), icon: Code2 },
  ];

  return (
    <WorkspacePage eyebrow="Workspace overview" title="Dashboard"
      description={'Welcome back' + (user.full_name ? ', ' + user.full_name : '') + '. Pick up where you left off.'}>
      <section aria-labelledby="workspace-status-heading" className="mb-8 overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
        <div className="flex flex-wrap items-center justify-between gap-3 border-b border-stone-200 px-5 py-4 sm:px-6">
          <div>
            <h2 id="workspace-status-heading" className="text-sm font-semibold text-stone-900">Workspace status</h2>
            <p className="mt-1 text-xs text-stone-500">{user.email}{user.job_role ? ' · ' + user.job_role : ''}</p>
          </div>
          <span className="rounded-md bg-stone-100 px-2 py-1 text-[11px] font-medium text-stone-600">Your setup</span>
        </div>
        <div className="grid gap-px bg-stone-100 sm:grid-cols-3">
          {statuses.map(item => <div key={item.label} className="bg-white px-5 py-5 sm:px-6">
            <div className="mb-5 flex h-9 w-9 items-center justify-center rounded-lg bg-stone-50 text-stone-600"><item.icon className="h-4 w-4" /></div>
            <p className="text-xs text-stone-500">{item.label}</p>
            <p className="mt-1 flex items-center gap-2 text-sm font-semibold text-stone-900">
              {item.complete ? <CheckCircle2 className="h-4 w-4 text-emerald-700" /> : <Circle className="h-4 w-4 text-amber-600" />}
              {item.value}
            </p>
          </div>)}
        </div>
      </section>

      <section aria-labelledby="next-steps-heading" className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm shadow-stone-200/40">
        <div className="border-b border-stone-200 px-5 py-4 sm:px-6">
          <h2 id="next-steps-heading" className="text-sm font-semibold text-stone-900">Work in Threadify</h2>
          <p className="mt-1 text-xs text-stone-500">Go straight to the parts of the workspace you use most.</p>
        </div>
        <div className="grid gap-px bg-stone-100 md:grid-cols-3">
          {steps.map(step => <Link key={step.href} to={step.href}
            className="group flex min-h-44 flex-col bg-white p-5 transition-colors hover:bg-stone-50 focus-visible:outline focus-visible:outline-2 focus-visible:outline-inset focus-visible:outline-emerald-700 sm:p-6">
            <span className="mb-6 flex h-10 w-10 items-center justify-center rounded-xl border border-stone-200 bg-stone-50 text-stone-700"><step.icon className="h-4 w-4" /></span>
            <span className="flex items-center gap-2 text-sm font-semibold text-stone-900">{step.title}<ArrowUpRight className="h-4 w-4 text-stone-400 transition-transform group-hover:-translate-y-0.5 group-hover:translate-x-0.5" /></span>
            <span className="mt-2 text-xs leading-5 text-stone-500">{step.description}</span>
          </Link>)}
        </div>
      </section>
    </WorkspacePage>
  );
}
