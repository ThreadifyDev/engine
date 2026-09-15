/** Browser credentials remain HttpOnly; only the session-bound CSRF value is readable. */
export function csrfToken(): string | null {
  if (typeof document === 'undefined') return null;
  for (const name of ['__Host-threadify_csrf', 'threadify_csrf_dev']) {
    const values = document.cookie.split(';').map(v => v.trim()).filter(v => v.startsWith(name + '='));
    if (values.length === 1) return values[0].slice(name.length + 1);
  }
  return null;
}

export function browserHeaders(): Record<string, string> {
  const csrf = csrfToken();
  return csrf ? { 'X-Threadify-CSRF': csrf } : {};
}

/** Old bearer tokens must not survive the move to revocable cookie sessions. */
export function purgeLegacyToken(): void {
  if (typeof window !== 'undefined') localStorage.removeItem('auth_token');
}
