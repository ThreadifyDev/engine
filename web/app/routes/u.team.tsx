import { useEffect, useState, type FormEvent } from 'react';
import type { MetaFunction } from '@remix-run/node';
import { api, type EngineUser } from '~/lib/api';
import AppLayout from '~/components/AppLayout';

export const meta: MetaFunction = () => [{ title: 'Team - Threadify' }];
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
  return <AppLayout><main className="p-8 space-y-6">
    <div><h1 className="text-2xl font-bold">Team</h1><p className="text-gray-600 mt-2">Manage invited, active, suspended and archived users.</p></div>
    {error && <p role="alert" className="rounded border border-red-200 bg-red-50 p-4 text-red-800">{error}</p>}
    {notice && <p role="status" className="rounded border border-green-200 bg-green-50 p-4 text-green-800">{notice}</p>}
    {canManage && <section className="rounded border p-5 space-y-4">
      <h2 className="font-semibold">Invite a user</h2>
      <p className="text-sm text-gray-600">An invited user becomes active after signing in with the matching email through Registry. Share the sign-in link with them.</p>
      <form onSubmit={invite} className="flex flex-wrap items-end gap-3">
        <label className="text-sm">Email<input aria-label="Email" type="email" required maxLength={255} value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} className="block rounded border p-2 mt-1" /></label>
        <label className="text-sm">Name<input aria-label="Name" maxLength={255} value={form.full_name} onChange={e => setForm({ ...form, full_name: e.target.value })} className="block rounded border p-2 mt-1" /></label>
        <label className="text-sm">Role<select aria-label="Invitation role" value={form.role} onChange={e => setForm({ ...form, role: e.target.value })} className="block rounded border p-2 mt-1">{roles.map(role => <option key={role}>{role}</option>)}</select></label>
        <button disabled={busy} className="rounded bg-black px-4 py-2 text-white disabled:opacity-50">Create invitation</button>
      </form>
      <div className="flex gap-2"><input aria-label="Sign-in link" readOnly value={loginUrl} className="min-w-0 flex-1 rounded border p-2 text-sm" /><button type="button" onClick={copyLink} className="rounded border px-3 py-2 text-sm">Copy sign-in link</button></div>
    </section>}
    <label className="block text-sm">Show users<select aria-label="User status filter" value={filter} onChange={e => setFilter(e.target.value)} className="ml-3 rounded border p-2">{['all', 'invited', 'active', 'suspended', 'archived'].map(status => <option key={status}>{status}</option>)}</select></label>
    {loading ? <p>Loading users…</p> : <div className="overflow-x-auto rounded border"><table className="w-full text-left text-sm">
      <thead className="bg-gray-50"><tr>{['Name', 'Email', 'Role', 'Status', 'Actions'].map(title => <th key={title} className="p-4">{title}</th>)}</tr></thead>
      <tbody>{shown.map(user => <tr key={user.id} className="border-t">
        <td className="p-4">{user.full_name || '—'}</td><td className="p-4">{user.email}</td>
        <td className="p-4">{canManage && user.status !== 'archived' ? <select aria-label={`Role for ${user.email}`} disabled={busy} value={user.roles[0] || ''} onChange={e => change(user, { role: e.target.value })} className="rounded border p-2">{roles.map(role => <option key={role}>{role}</option>)}</select> : user.roles.join(', ')}</td>
        <td className="p-4">{user.status}</td><td className="p-4">{canManage && user.status !== 'archived' && <div className="flex gap-3">
          {user.status === 'active' && <button disabled={busy} onClick={() => change(user, { status: 'suspended' })} className="underline">Suspend</button>}
          {user.status === 'suspended' && <button disabled={busy} onClick={() => change(user, { status: 'active' })} className="underline">Reactivate</button>}
          <button disabled={busy} onClick={() => { if (window.confirm(`Archive ${user.email}? This ends their access and cannot be undone.`)) void change(user, { status: 'archived' }); }} className="text-red-700 underline">{user.status === 'invited' ? 'Cancel invitation' : 'Archive'}</button>
        </div>}</td>
      </tr>)}</tbody>
    </table>{shown.length === 0 && <p className="p-6 text-gray-500">No users with this status.</p>}</div>}
  </main></AppLayout>;
}
