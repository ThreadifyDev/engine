type Transaction = { transaction_id: string; poll_token: string; expires_at: string };
type Poll = (id: string, token: string, signal: AbortSignal) => Promise<{ status: string }>;

export function managedLoginURL(verificationURL: string, reauthenticate: boolean): string {
  const url = new URL(verificationURL);
  if (reauthenticate) url.searchParams.set('reauthenticate', 'true');
  return url.toString();
}

function pause(signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    const aborted = () => { clearTimeout(timer); reject(signal.reason); };
    const timer = setTimeout(() => { signal.removeEventListener('abort', aborted); resolve(); }, 1500);
    signal.addEventListener('abort', aborted, { once: true });
    if (signal.aborted) { signal.removeEventListener('abort', aborted); aborted(); }
  });
}

// The Registry closes its tab after approval. Only the Engine response establishes a session.
export async function waitForManagedLogin(transaction: Transaction, signal: AbortSignal, poll: Poll, wait = pause) {
  let failures = 0;
  while (Date.now() < Date.parse(transaction.expires_at)) {
    signal.throwIfAborted();
    try {
      const result = await poll(transaction.transaction_id, transaction.poll_token, signal);
      signal.throwIfAborted();
      failures = 0;
      if (result.status === 'authenticated') return;
    } catch (error) {
      signal.throwIfAborted();
      const code = error instanceof Error ? error.message : '';
      if (code.endsWith('_denied') || ++failures >= 3) throw error;
    }
    await wait(signal);
  }
  throw new Error('managed_login_expired');
}

// A common public outcome does not disclose identity, membership or invitation state.
export const MANAGED_SIGN_IN_ERROR = 'We couldn’t complete sign-in. Revalidate your sign-in to try again.';
