import assert from 'node:assert/strict';
import test from 'node:test';
import { build } from 'esbuild';

const output = await build({
  entryPoints: ['app/lib/api-key-creation.ts'],
  bundle: true,
  write: false,
  format: 'esm',
  platform: 'node',
});
const { apiKeyExpiryParams, availableServiceAccounts, defaultServiceAccountSelection } = await import(
  `data:text/javascript;base64,${Buffer.from(output.outputFiles[0].text).toString('base64')}`
);

test('custom expiry preserves the selected local calendar day across time zones', () => {
  const originalTimezone = process.env.TZ;
  try {
    for (const timezone of ['America/Los_Angeles', 'Europe/London', 'Asia/Tokyo']) {
      process.env.TZ = timezone;
      const { expires_at } = apiKeyExpiryParams('custom', '2031-06-15');
      const expiry = new Date(expires_at);
      assert.equal(expiry.getFullYear(), 2031, timezone);
      assert.equal(expiry.getMonth(), 5, timezone);
      assert.equal(expiry.getDate(), 15, timezone);
      assert.equal(expiry.getHours(), 23, timezone);
      assert.equal(expiry.getMinutes(), 59, timezone);
    }
  } finally {
    process.env.TZ = originalTimezone;
  }
});

test('expiry presets stay supported and invalid custom dates are rejected', () => {
  assert.deepEqual(apiKeyExpiryParams('never', ''), {});
  assert.deepEqual(apiKeyExpiryParams('90', ''), { expires_in: 90 });
  assert.throws(() => apiKeyExpiryParams('custom', ''), /Choose an expiry date/);
  assert.throws(() => apiKeyExpiryParams('custom', '2020-01-01'), /future/);
  assert.throws(() => apiKeyExpiryParams('custom', '2031-02-30'), /valid expiry date/);
});

test('only active service accounts are offered for existing-account selection', () => {
  const accounts = [
    { id: 'inactive', is_active: false },
    { id: 'existing', is_active: true },
  ];
  assert.deepEqual(availableServiceAccounts(accounts), [accounts[1]]);
  assert.deepEqual(availableServiceAccounts([]), []);
  assert.deepEqual(defaultServiceAccountSelection(accounts), { createNew: false, selectedId: 'existing' });
  assert.deepEqual(defaultServiceAccountSelection([]), { createNew: true, selectedId: '' });
});
