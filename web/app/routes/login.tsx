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

  return <main className="flex min-h-screen items-center justify-center bg-[#f8f8f6] px-4 py-12">
    <div className="w-full max-w-md">
      <header className="mb-8 text-center">
        <p className="mb-2 text-[11px] font-semibold uppercase tracking-[0.18em] text-emerald-700">Threadify / Engine access</p>
        <h1 className="text-3xl font-semibold tracking-tight text-stone-950">Welcome to Threadify</h1>
        <p className="mt-3 text-sm leading-6 text-stone-500">Sign in to manage your workflows and their activity.</p>
      </header>
      <section className="overflow-hidden rounded-2xl border border-stone-200 bg-white shadow-sm">
        <div className="border-b border-stone-100 px-6 py-5 sm:px-8"><h2 className="text-lg font-semibold tracking-tight text-stone-900">Sign in to your Engine</h2><p className="mt-1 text-sm text-stone-500">Use your Fused account with email or SSO.</p></div>
        <div className="space-y-5 px-6 py-6 sm:px-8">
          {error && <p role="alert" className="rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">{error}</p>}
          <button type="button" disabled={busy} onClick={() => managed()} className="w-full rounded-lg bg-stone-900 px-4 py-3 text-sm font-medium text-white transition hover:bg-stone-700 disabled:opacity-50">
            {busy ? 'Waiting for sign-in…' : managedRetry ? 'Revalidate sign-in' : 'Continue with email or SSO'}
          </button>
          {busy && popup.current && <button type="button" onClick={() => controller.current?.abort()} className="text-sm font-medium text-stone-600 underline hover:text-stone-900">Cancel sign-in</button>}
          <div className="border-t border-stone-100 pt-5">
            <form onSubmit={exchange} className="space-y-3">
              <label htmlFor="api-key" className="block text-sm font-medium text-stone-700">Or sign in with an API key</label>
              <input id="api-key" type="password" autoComplete="off" required value={key} onChange={e => setKey(e.target.value)} className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100" placeholder="Enter API key" />
              <button disabled={busy || !key.trim()} className="w-full rounded-lg border border-stone-200 bg-white px-4 py-3 text-sm font-medium text-stone-700 transition hover:bg-stone-50 disabled:opacity-50">Sign in with API key</button>
            </form>
          </div>
        </div>
      </section>
    </div>
  </main>;
}
