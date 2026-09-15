import { json, type ActionFunctionArgs, type LoaderFunctionArgs } from '@remix-run/node';
import { getConfig } from '~/config.server';

const ALLOWED_PATH = /^(?:agent|responses|sessions(?:\/[^/]+(?:\/messages)?)?)$/;
const ALLOWED_METHODS = new Set(['GET', 'POST', 'PATCH', 'DELETE']);

async function proxyHarnest(request: Request, splat: string | undefined) {
  const path = splat?.replace(/^\/+|\/+$/g, '') || '';
  if (!ALLOWED_PATH.test(path) || !ALLOWED_METHODS.has(request.method)) {
    return json({ detail: 'Unsupported Harnest route' }, { status: 404 });
  }

  // Validate the HttpOnly session and its CSRF binding at the Engine before proxying.
  const cookie = request.headers.get('cookie') || '';
  const tokens = cookie.split(';').map(v => v.trim()).filter(v => /^(?:__Host-threadify_session|threadify_session_dev)=/.test(v));
  if (tokens.length !== 1 || request.headers.has('authorization')) {
    return json({ detail: 'Please sign in to Threadify' }, { status: 401 });
  }
  const authorization = `Bearer ${tokens[0].slice(tokens[0].indexOf('=') + 1)}`;
  const sourceUrl = new URL(request.url);
  const targetUrl = new URL(`/${path}`, getConfig().harnestUrl);
  targetUrl.search = sourceUrl.search;

  try {
    const verified = await fetch(`${getConfig().engineUrl.replace(/\/+$/, '')}/auth/session/verify`, {
      method: 'POST', redirect: 'error', signal: request.signal,
      headers: { Cookie: cookie, Origin: request.headers.get('origin') || sourceUrl.origin,
        'X-Threadify-CSRF': request.headers.get('x-threadify-csrf') || '', 'Content-Type': 'application/json' }, body: '{}',
    });
    if (!verified.ok) return json({ detail: 'Threadify session verification failed' }, { status: verified.status });
    const upstream = await fetch(targetUrl, {
      method: request.method,
      redirect: 'error',
      headers: {
        Accept: request.headers.get('accept') || 'application/json',
        Authorization: authorization,
        ...(request.headers.get('content-type')
          ? { 'Content-Type': request.headers.get('content-type')! }
          : {}),
      },
      body:
        request.method === 'GET' || request.method === 'DELETE'
          ? undefined
          : await request.arrayBuffer(),
      signal: request.signal,
    });

    const headers = new Headers();
    const contentType = upstream.headers.get('content-type');
    if (contentType) headers.set('Content-Type', contentType);
    headers.set('Cache-Control', 'no-store');

    return new Response(upstream.body, {
      status: upstream.status,
      statusText: upstream.statusText,
      headers,
    });
  } catch (error) {
    console.error('[Harnest proxy] Agent request failed', error);
    return json(
      { detail: 'The Threadify agent is currently unavailable' },
      { status: 503 }
    );
  }
}

export async function loader({ request, params }: LoaderFunctionArgs) {
  return proxyHarnest(request, params['*']);
}

export async function action({ request, params }: ActionFunctionArgs) {
  return proxyHarnest(request, params['*']);
}
