import { useEffect, useState, type FormEvent } from 'react';
import { api, type EngineSettings } from '~/lib/api';

/** The public address belongs to this installation, with a config default and a durable admin override. */
export function EngineTab() {
  const [settings, setSettings] = useState<EngineSettings | null>(null);
  const [url, setUrl] = useState('');
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [notice, setNotice] = useState('');
  function show(result: EngineSettings) { setSettings(result); setUrl(result.public_url); }
  useEffect(() => {
    let mounted = true;
    api.getEngineSettings().then(result => { if (mounted) show(result); })
      .catch(() => { if (mounted) setError('Could not load Engine settings.'); });
    return () => { mounted = false; };
  }, []);
  async function save(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError(''); setNotice('');
    try { show(await api.saveEnginePublicURL(url)); setNotice('Public Engine URL saved.'); }
    catch (e) { setError(e instanceof Error && e.message === 'invalid_public_url' ? 'Enter an HTTP or HTTPS URL without credentials, a query or a fragment.' : 'Could not save the public Engine URL. Administrator access is required.'); }
    finally { setBusy(false); }
  }
  async function reset() {
    setBusy(true); setError(''); setNotice('');
    try { show(await api.resetEnginePublicURL()); setNotice('Using the URL from config.yaml.'); }
    catch { setError('Could not restore the config default.'); }
    finally { setBusy(false); }
  }
  return <section className="max-w-3xl space-y-5 rounded-2xl border border-stone-200 bg-white p-6 text-sm leading-6 shadow-sm sm:p-8">
    <h3 className="border-b border-stone-100 pb-4 text-lg font-semibold tracking-tight text-stone-900">Engine URL</h3>
    <p className="text-gray-600">Use one address to connect your SDK. Threadify handles writing and querying threads automatically, including any reverse-proxy path.</p>
    {error && <p role="alert" className="rounded border border-red-200 bg-red-50 p-3 text-red-800">{error}</p>}
    {notice && <p role="status" className="rounded border border-green-200 bg-green-50 p-3 text-green-800">{notice}</p>}
    {!settings ? <p>Loading Engine settings…</p> : <>
      <form onSubmit={save} className="space-y-3">
        <label htmlFor="public-engine-url" className="block text-sm font-medium">Engine URL</label>
        <input id="public-engine-url" type="url" required maxLength={2048} placeholder="https://threadify.example.com" value={url} disabled={!settings.can_manage || busy} onChange={e => setUrl(e.target.value)} className="w-full rounded-lg border border-stone-200 px-3 py-2.5 text-sm outline-none focus:border-emerald-400 focus:ring-2 focus:ring-emerald-100" />
        {settings.can_manage && <div className="flex gap-3">
          <button disabled={busy || !url.trim()} className="rounded-lg bg-stone-900 px-4 py-2.5 font-medium text-white hover:bg-stone-700 disabled:opacity-50">Save Engine URL</button>
          <button type="button" disabled={busy || settings.source !== 'ui'} onClick={reset} className="rounded-lg border border-stone-200 px-4 py-2.5 font-medium text-stone-700 hover:bg-stone-50 disabled:opacity-50">Use config default</button>
        </div>}
      </form>
      <p className="text-[13px] text-gray-600">{settings.source === 'ui' ? 'Using the saved UI setting. It persists across restarts.' : settings.source === 'config' ? 'Using server.public_url from config.yaml.' : 'No public URL is configured.'}</p>
      <p className="text-[13px] text-gray-600">Config default: <code>{settings.config_public_url || 'Not set'}</code></p>
      <p className="text-[13px] text-gray-600">This sets the advertised address. Keep the UI’s deployment connection and reverse proxy configured separately.</p>
      {settings.public_url && <>
        <div className="space-y-3 rounded-xl border border-stone-200 bg-stone-50 p-4">
          <h4 className="font-semibold">Connect with the JavaScript SDK</h4>
          <pre className="overflow-x-auto rounded bg-gray-50 p-3 text-xs"><code>{`const connection = await Threadify.connect(\n  process.env.THREADIFY_API_KEY,\n  "my-service",\n  { engineUrl: ${JSON.stringify(settings.public_url)} }\n);`}</code></pre>
        </div>
        <details className="rounded-xl border border-stone-200 p-4">
          <summary className="cursor-pointer font-semibold">OpenTelemetry setup</summary>
          <p className="my-3 text-sm text-gray-600">For an OTLP/HTTP exporter, set OTEL_EXPORTER_OTLP_TRACES_ENDPOINT to this address and configure its API-key authorization header.</p>
          <input aria-label="OTEL traces endpoint" readOnly value={settings.endpoints.otel || ''} className="w-full rounded border bg-gray-50 p-2 font-mono text-xs" />
        </details>
        <details className="rounded-xl border border-stone-200 p-4">
          <summary className="cursor-pointer font-semibold">MCP setup</summary>
          <p className="my-3 text-sm text-gray-600">Use this server URL and your service-account API key in your MCP client.</p>
          <input aria-label="MCP endpoint" readOnly value={settings.endpoints.mcp || ''} className="w-full rounded border bg-gray-50 p-2 font-mono text-xs" />
        </details>
      </>}
    </>}
  </section>;
}
