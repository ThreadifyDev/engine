import { useEffect, useState } from 'react';
import { api } from '~/lib/api';
import { getConfig } from '~/config.client';

export default function CLILogin() {
  const [transaction, setTransaction] = useState<{ id: string; token: string } | null>(null);
  const [identity, setIdentity] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [approved, setApproved] = useState(false);

  useEffect(() => {
    let active = true;
    const fragment = new URLSearchParams(window.location.hash.slice(1));
    const id = fragment.get('transaction_id');
    const token = fragment.get('browser_token');
    if (!id || !token) { setError('This login link is incomplete. Run threadify login again.'); return; }
    api.session().then(session => {
      if (!active) return;
      if (!session.authenticated) {
        window.location.replace('/login?next=cli-login' + window.location.hash); return;
      }
      setTransaction({ id, token });
      setIdentity(session.user?.email || session.user?.full_name || 'your current account');
    }).catch(() => { if (active) setError('Could not verify your session. Sign in and try again.'); });
    return () => { active = false; };
  }, []);

  async function approve() {
    if (!transaction) return;
    setBusy(true); setError('');
    try {
      await api.approveCLILogin(transaction.id, transaction.token);
      setApproved(true); setTransaction(null);
      window.history.replaceState(null, '', '/cli-login');
    } catch { setError('Approval failed or expired. Check the Engine address and run threadify login again.'); }
    finally { setBusy(false); }
  }

  return <main className="min-h-screen bg-white flex items-center justify-center px-4 text-sm">
    <div className="w-full max-w-md rounded-xl border p-7 space-y-5">
      <h1 className="text-xl font-semibold">{approved ? 'CLI approved' : 'Approve Threadify CLI'}</h1>
      {approved ? <p>Return to your terminal. You can close this tab.</p> : <>
        <p>Allow the CLI to access <strong>{getConfig().engineUrl}</strong> as <strong>{identity || '…'}</strong> with your current permissions for up to 30 days.</p>
        <p className="text-gray-600">Approve only if you just ran <code>threadify login</code>. Your CLI credential stays on that computer. Run <code>threadify logout</code> to revoke it.</p>
        {transaction && <p className="text-gray-500">Request: <code>{transaction.id.slice(0, 12)}</code></p>}
        <div className="flex gap-3">
          <button type="button" onClick={approve} disabled={!transaction || busy} className="rounded bg-black px-4 py-2 text-white disabled:opacity-50">{busy ? 'Approving…' : 'Approve CLI login'}</button>
          <a href="/u/dashboard" className="rounded border px-4 py-2">Cancel</a>
        </div>
      </>}
      {error && <p role="alert" className="text-red-700">{error}</p>}
    </div>
  </main>;
}
