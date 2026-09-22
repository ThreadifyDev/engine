import { useEffect, useRef, useState, type FormEvent } from 'react';
import { api } from '~/lib/api';
import { purgeLegacyToken } from '~/lib/browser-session';
import { MANAGED_SIGN_IN_ERROR, managedLoginURL, waitForManagedLogin } from '~/lib/managed-login';

/** Both login methods exchange authority for an Engine-owned HttpOnly session. */
function afterLogin() {
  const current = new URL(window.location.href);
  return current.searchParams.get('next') === 'cli-login' ? '/cli-login' + current.hash : '/u/dashboard';
}

export default function Login() {
  const [key, setKey] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [managedRetry, setManagedRetry] = useState(false);
  const controller = useRef<AbortController | null>(null);
  const popup = useRef<Window | null>(null);

  useEffect(() => {
    purgeLegacyToken();
    let mounted = true;
    api.session().then(s => { if (mounted && s.authenticated) window.location.replace(afterLogin()); }).catch(() => {});
    return () => { mounted = false; controller.current?.abort(); popup.current?.close(); };
  }, []);

  async function exchange(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError('');
    try { await api.exchangeAPIKey(key.trim()); setKey(''); window.location.replace(afterLogin()); }
    catch { setError('The API key was not accepted for this Engine.'); }
    finally { setBusy(false); }
  }

  function failManagedLogin() {
    setError(MANAGED_SIGN_IN_ERROR);
    setManagedRetry(true);
  }

  async function managed() {
    const tab = window.open('about:blank', '_blank');
    if (!tab) { failManagedLogin(); return; }
    try { tab.opener = null; }
    catch { tab.close(); failManagedLogin(); return; }
    popup.current = tab;
    const abort = new AbortController(); controller.current?.abort(); controller.current = abort;
    setBusy(true); setError('');
    try {
      const transaction = await api.startManagedLogin();
      if (abort.signal.aborted) return;
      tab.location.replace(managedLoginURL(transaction.verification_url, managedRetry));
      await waitForManagedLogin(transaction, abort.signal, api.pollManagedLogin.bind(api));
      const session = await api.session();
      if (!session.authenticated) throw new Error('managed_login_internal_error');
      if (!abort.signal.aborted) window.location.replace(afterLogin());
    } catch { if (!abort.signal.aborted) failManagedLogin(); }
    finally { tab.close(); popup.current = null; setBusy(false); }
  }

  return <main className="min-h-screen bg-white flex items-center justify-center px-4">
    <div className="w-full max-w-md space-y-6">
      <h1 className="text-3xl font-bold text-center">Threadify</h1>
      <div className="rounded-xl border p-8 space-y-5">
        <h2 className="text-xl font-semibold">Sign in to your Engine</h2>
        <p className="text-sm text-gray-600">Use your Fused account with email or SSO.</p>
        {error && <p role="alert" className="text-sm text-red-700">{error}</p>}
        <button type="button" disabled={busy} onClick={() => managed()} className="w-full rounded bg-black px-4 py-3 text-white disabled:opacity-50">
          {busy ? 'Waiting for sign-in…' : managedRetry ? 'Revalidate sign-in' : 'Continue with email or SSO'}
        </button>
        {busy && popup.current && <button type="button" onClick={() => controller.current?.abort()} className="text-sm text-gray-600 underline">Cancel sign-in</button>}
        <div className="border-t pt-5">
          <form onSubmit={exchange} className="space-y-3">
            <label htmlFor="api-key" className="block text-sm font-medium">API key</label>
            <input id="api-key" type="password" autoComplete="off" required value={key} onChange={e => setKey(e.target.value)} className="w-full rounded border p-3" />
            <button disabled={busy || !key.trim()} className="w-full rounded border px-4 py-3 disabled:opacity-50">Sign in with API key</button>
          </form>
        </div>
      </div>
    </div>
  </main>;
}
