import { useEffect, useState, type FormEvent } from 'react';
import { api, type EngineUser } from '~/lib/api';
import WorkspacePage from '~/components/WorkspacePage';
import { Copy, Mail, Users } from 'lucide-react';

const roles = ['admin', 'member', 'viewer'];
const errorMessages: Record<string, string> = {
  last_active_admin_required: 'Keep at least one active administrator.',
  user_state_conflict: 'This user already exists or cannot make that state change. Manage the existing user below.',
  user_management_denied: 'You do not have permission to manage users.',
  invalid_user: 'Enter a valid email address and select a role.',
};

/** The Engine owns invitations and user states; verified Registry sign-in activates an invitation. */
export default function Team() {
  const [users, setUsers] = useState<EngineUser[]>([]);
  const [canManage, setCanManage] = useState(false);
  const [loginUrl, setLoginUrl] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  const [filter, setFilter] = useState('all');
  const [form, setForm] = useState({ email: '', full_name: '', role: 'member' });

  function showError(e: unknown) {
    const message = e instanceof Error ? e.message : 'The request failed.';
    setError(errorMessages[message] || message);
  }
  async function refresh() {
    const result = await api.listEngineUsers();
    setUsers(result.users); setCanManage(result.can_manage); setLoginUrl(result.login_url);
  }
  useEffect(() => {
    let mounted = true;
    api.listEngineUsers().then(result => {
      if (mounted) { setUsers(result.users); setCanManage(result.can_manage); setLoginUrl(result.login_url); }
    }).catch(e => { if (mounted) showError(e); }).finally(() => { if (mounted) setLoading(false); });
    return () => { mounted = false; };
  }, []);

  async function invite(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(''); setNotice('');
    try {
      const result = await api.inviteEngineUser(form);
      setLoginUrl(result.login_url);
      setNotice(`${result.user.email} is invited. Share the sign-in link below; they must sign in with that email.`);
      setForm({ email: '', full_name: '', role: 'member' }); await refresh();
    } catch (e) { showError(e); } finally { setBusy(false); }
  }
  async function change(user: EngineUser, data: { role?: string; status?: EngineUser['status'] }) {
    setBusy(true); setError(''); setNotice('');
    try { await api.updateEngineUser(user.id, data); await refresh(); setNotice(`${user.email} updated.`); }
    catch (e) { showError(e); } finally { setBusy(false); }
  }
  async function copyLink() {
    try { await navigator.clipboard.writeText(loginUrl); setNotice('Sign-in link copied.'); }
    catch { setError('Copy the sign-in link from the field below.'); }
  }
  const shown = users.filter(user => filter === 'all' || user.status === filter);
  const fieldClass = 'mt-2 block w-full rounded-lg border border-stone-200 bg-white px-3 py-2.5 text-sm text-stone-900 outline-none transition focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100';
  const statusClass: Record<string, string> = {
    active: 'bg-emerald-50 text-emerald-700 ring-emerald-100',
    invited: 'bg-amber-50 text-amber-700 ring-amber-100',
    suspended: 'bg-red-50 text-red-700 ring-red-100',
    archived: 'bg-stone-100 text-stone-600 ring-stone-200',
  };

  return (
    <WorkspacePage eyebrow="Workspace / Access" title="Team" description="Invite people and manage access to this Engine.">
      <div className="space-y-5">
        {error && <p role="alert" className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-800">{error}</p>}
        {notice && <p role="status" className="rounded-xl border border-emerald-200 bg-emerald-50 px-4 py-3 text-sm text-emerald-800">{notice}</p>}

        {canManage && (
          <section className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
            <div className="border-b border-stone-100 px-6 py-5">
              <div className="flex items-center gap-2"><Mail className="h-4 w-4 text-emerald-700" /><h2 className="text-lg font-semibold tracking-tight text-stone-900">Invite a teammate</h2></div>
              <p className="mt-1 text-sm text-stone-500">They can activate their invitation by signing in with the email you enter.</p>
            </div>
            <div className="space-y-5 p-6">
              <form onSubmit={invite} className="grid gap-4 lg:grid-cols-[minmax(0,1.2fr)_minmax(0,1fr)_minmax(8rem,.5fr)_auto] lg:items-end">
                <label className="text-xs font-semibold uppercase tracking-wide text-stone-500">Email address<input aria-label="Email" type="email" required maxLength={255} value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} className={fieldClass} placeholder="teammate@example.com" /></label>
                <label className="text-xs font-semibold uppercase tracking-wide text-stone-500">Full name<input aria-label="Name" maxLength={255} value={form.full_name} onChange={e => setForm({ ...form, full_name: e.target.value })} className={fieldClass} placeholder="Optional" /></label>
                <label className="text-xs font-semibold uppercase tracking-wide text-stone-500">Role<select aria-label="Invitation role" value={form.role} onChange={e => setForm({ ...form, role: e.target.value })} className={fieldClass}>{roles.map(role => <option key={role}>{role}</option>)}</select></label>
                <button disabled={busy} className="inline-flex h-[42px] items-center justify-center rounded-lg bg-stone-900 px-4 text-sm font-medium text-white transition hover:bg-stone-700 disabled:opacity-50">Create invitation</button>
              </form>
              <div className="rounded-xl border border-stone-200 bg-stone-50 p-4">
                <label htmlFor="team-sign-in-link" className="text-xs font-semibold uppercase tracking-wide text-stone-500">Team sign-in link</label>
                <div className="mt-2 flex flex-col gap-2 sm:flex-row">
                  <input id="team-sign-in-link" aria-label="Sign-in link" readOnly value={loginUrl} className="min-w-0 flex-1 rounded-lg border border-stone-200 bg-white px-3 py-2.5 font-mono text-xs text-stone-600" />
                  <button type="button" onClick={copyLink} disabled={!loginUrl} className="inline-flex items-center justify-center gap-2 rounded-lg border border-stone-200 bg-white px-4 py-2 text-sm font-medium text-stone-700 transition hover:bg-stone-100 disabled:opacity-50"><Copy className="h-4 w-4" />Copy link</button>
                </div>
              </div>
            </div>
          </section>
        )}

        <section className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
          <div className="flex flex-wrap items-center justify-between gap-4 border-b border-stone-100 px-6 py-5">
            <div><h2 className="flex items-center gap-2 text-lg font-semibold tracking-tight text-stone-900"><Users className="h-4 w-4 text-emerald-700" /> Members <span className="rounded-full bg-stone-100 px-2.5 py-1 text-xs font-medium text-stone-600">{users.length}</span></h2><p className="mt-1 text-sm text-stone-500">Review roles and account status.</p></div>
            <label className="flex items-center gap-3 text-xs font-semibold uppercase tracking-wide text-stone-500">Status<select aria-label="User status filter" value={filter} onChange={e => setFilter(e.target.value)} className="rounded-lg border border-stone-200 bg-white px-3 py-2 text-sm font-normal capitalize text-stone-700 outline-none focus:border-emerald-400">{['all', 'invited', 'active', 'suspended', 'archived'].map(status => <option key={status} value={status}>{status === 'all' ? 'All statuses' : status}</option>)}</select></label>
          </div>
          {loading ? <p role="status" className="px-6 py-12 text-center text-sm text-stone-500">Loading members…</p> : error ? <p className="px-6 py-10 text-center text-sm text-stone-500">Members could not be loaded.</p> : shown.length === 0 ? <div className="px-6 py-14 text-center"><div className="mx-auto mb-4 flex h-12 w-12 items-center justify-center rounded-2xl bg-emerald-50"><Users className="h-6 w-6 text-emerald-700" /></div><p className="font-medium text-stone-900">No members in this view</p><p className="mt-1 text-sm text-stone-500">Try another status or invite a teammate.</p></div> : (
            <div className="overflow-x-auto"><table className="w-full min-w-[680px] text-left text-sm"><thead className="bg-stone-50 text-xs font-semibold uppercase tracking-wide text-stone-500"><tr>{['Member', 'Role', 'Status', 'Actions'].map(title => <th key={title} className="px-6 py-3">{title}</th>)}</tr></thead><tbody className="divide-y divide-stone-100">{shown.map(user => <tr key={user.id} className="hover:bg-stone-50/70">
              <td className="px-6 py-4"><div className="font-medium text-stone-900">{user.full_name || user.email}</div><div className="mt-0.5 text-xs text-stone-500">{user.email}</div></td>
              <td className="px-6 py-4">{canManage && user.status !== 'archived' ? <select aria-label={`Role for ${user.email}`} disabled={busy} value={user.roles[0] || ''} onChange={e => change(user, { role: e.target.value })} className="rounded-lg border border-stone-200 bg-white px-2.5 py-2 text-sm capitalize text-stone-700 outline-none focus:border-emerald-400 disabled:opacity-50">{roles.map(role => <option key={role}>{role}</option>)}</select> : <span className="capitalize text-stone-600">{user.roles.join(', ')}</span>}</td>
              <td className="px-6 py-4"><span className={`inline-flex rounded-full px-2.5 py-1 text-xs font-medium capitalize ring-1 ring-inset ${statusClass[user.status] || statusClass.archived}`}>{user.status}</span></td>
              <td className="px-6 py-4">{canManage && user.status !== 'archived' && <div className="flex flex-wrap gap-3 text-xs font-medium">{user.status === 'active' && <button disabled={busy} onClick={() => change(user, { status: 'suspended' })} className="text-stone-700 hover:text-stone-900 disabled:opacity-50">Suspend</button>}{user.status === 'suspended' && <button disabled={busy} onClick={() => change(user, { status: 'active' })} className="text-emerald-700 hover:text-emerald-900 disabled:opacity-50">Reactivate</button>}<button disabled={busy} onClick={() => { if (window.confirm(`Archive ${user.email}? This ends their access and cannot be undone.`)) void change(user, { status: 'archived' }); }} className="text-red-700 hover:text-red-900 disabled:opacity-50">{user.status === 'invited' ? 'Cancel invitation' : 'Archive'}</button></div>}</td>
            </tr>)}</tbody></table></div>
          )}
        </section>
      </div>
    </WorkspacePage>
  );
}
