export type ServiceAccountChoice = { id: string; is_active?: boolean };

export function availableServiceAccounts<T extends ServiceAccountChoice>(accounts: T[]): T[] {
  return accounts.filter(account => account.is_active !== false);
}

export function defaultServiceAccountSelection(accounts: ServiceAccountChoice[]): { createNew: boolean; selectedId: string } {
  const firstAccount = availableServiceAccounts(accounts)[0];
  return { createNew: !firstAccount, selectedId: firstAccount?.id || '' };
}

export function localToday(): string {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, '0');
  const day = String(now.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

export function apiKeyExpiryParams(expiresIn: string, customDate: string): { expires_in?: number; expires_at?: string } {
  if (expiresIn === 'never') return {};
  if (expiresIn !== 'custom') return { expires_in: Number(expiresIn) };

  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(customDate);
  if (!match) throw new Error('Choose an expiry date.');
  const [, year, month, day] = match.map(Number);
  const expiry = new Date(year, month - 1, day, 23, 59, 59, 999);
  if (expiry.getFullYear() !== year || expiry.getMonth() !== month - 1 || expiry.getDate() !== day) {
    throw new Error('Choose a valid expiry date.');
  }
  if (expiry.getTime() <= Date.now()) throw new Error('Expiry date must be in the future.');
  return { expires_at: expiry.toISOString() };
}
