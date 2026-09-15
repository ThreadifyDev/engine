# Registry browser authentication

Threadify uses the Fused Registry identity flow. A license can enable Threadify,
Fused, or both; Threadify sign-in checks the Threadify entitlement independently.
Supabase, local passwords, OTP endpoints, and browser JWT storage are retired from
the production login path. Existing password/verification UI routes redirect to
`/login`; `/api/auth/*` returns HTTP 410.

## Configuration and deployment

The Engine and external Web API share PostgreSQL, `registry.license_key`, and
`registry.installation_id`. Their persisted installation ID is reused when omitted.
Configure both with the exact UI origin:

```yaml
registry:
  license_key: "$THREADIFY_LICENSE_KEY"
  browser_origin: "$THREADIFY_BROWSER_ORIGIN:"
```

Use HTTPS in production, for example `THREADIFY_BROWSER_ORIGIN=https://threadify.example.com`.
Behind a reverse proxy this setting is required; forwarded host/protocol headers
are not trusted. Route `/auth/*`, `/graphql` and `/v1/*` to the Engine, `/api/*` to
the external Web API, and UI pages/assets to the UI. All three should share one
public hostname. Local development may use HTTP on loopback and different ports
on the same hostname (UI 3002, Engine 8083, Web API 3003).

The Registry deployment needs the Threadify identity adapter and its existing
managed identity/Logto configuration. The adapter registers the Threadify
installation in shared identity storage and reuses the existing provider flow;
it does not enable Fused product access. A Registry without managed identity can
still support API-key exchange, but email/SSO sign-in is unavailable.

## Sign-in and permissions

- **Email or SSO:** The Engine creates a Registry transaction with a server-only
  verifier. The browser opens its verification URL and polls using a separate
  capability bound to an HttpOnly login cookie. The Engine exchanges the completed
  transaction, validates account/installation/provider binding, and consumes it once.
- **API key:** `/auth/api-key/exchange` resolves an active local API key. User keys
  retain the user's roles; service keys retain the service principal's roles.
  The configured Registry license is administrator bootstrap authority for its owner.
- **Membership:** The Engine owns users, roles and invitation state. A first
  Registry sign-in binds the license owner or a local `invited` user whose email
  matches the verified assertion. Later sign-ins require the bound user to remain
  active. A Registry account alone does not grant installation membership.

Sessions expire after eight hours. PostgreSQL stores token hashes, with provider
logout capabilities and pending verifiers encrypted using a key derived from the
license and installation. Browser JavaScript receives no session token. Cookies
are HttpOnly, host-only, SameSite=Strict, and Secure with `__Host-` names on HTTPS.
A readable, session-bound CSRF cookie must match `X-Threadify-CSRF` on mutations,
which also require the configured Origin. Development cookie names end in `_dev`.

Every session request checks current membership, key revocation/expiry, service
account activity and Registry verification state. Logout revokes the local session
before requesting provider logout. Engine restarts preserve sessions; expiry,
explicit logout, membership removal or source-key revocation removes access.
Rotating the license changes cookie/encryption keys and requires browser sign-in
again. SDK/API-key transport remains available independently of browser cookies.

## Engine-owned users and invitations

Open **Team** to create an invitation with an email, name and `admin`, `member`
or `viewer` role. The Engine immediately stores the user with status `invited`.
Share the displayed UI sign-in link. The Engine does not send an invitation email
or create a legacy Web API invitation/outbox job. Registry verifies identity;
the Engine binds that identity and changes the invited user to `active`.

| Status | Access and transitions |
| --- | --- |
| `invited` | No access, including through personal keys. Verified Registry sign-in activates the user; an admin can change the role or archive the invitation. |
| `active` | Current roles apply. An admin can change the role, suspend or archive the user. |
| `suspended` | Browser sessions and personal keys are blocked. An admin can reactivate or archive the user. Reactivation requires a new browser session. |
| `archived` | Permanent end of access. Personal keys are revoked; the identity cannot be reactivated or reinvited. |

The Engine preserves at least one active human administrator, including during
concurrent updates. Role and status changes revoke existing browser sessions.
Service accounts retain their own lifecycle; archiving their creator does not
revoke service-account keys.

Engine endpoints accept browser sessions with CSRF protection or an API key with
the required local role:

- `GET /v1/users`: list local users; returns `users`, `can_manage` and `login_url`.
- `POST /v1/users`: create an invitation with `{ "email": "person@example.com", "full_name": "Person", "role": "member" }`.
- `PATCH /v1/users/:id`: change `full_name`, `role` or `status`.

Only administrators can mutate users. Repeating an identical pending invitation
returns the same user. Invitations do not expire automatically; cancel one by
archiving it. `/api/team` and `/api/team/*` return HTTP 410. Existing user rows
keep active status on upgrade; legacy pending invitation rows are not imported.

## Verification

```sh
cd threadify-go/shared
THREADIFY_AUTH_TEST_DATABASE_URL='postgres://...' go test ./auth ./registry
```

The auth suite creates and removes isolated PostgreSQL schemas. It covers cookie
security, CSRF, cross-origin rejection, replica session sharing, key authority,
revocation, expiry, invitation activation, suspension, archival, last-admin
protection, cross-company isolation, Registry assertion binding, concurrent one-use
polling, encrypted capabilities and provider logout. Registry adapter tests live
in the Fused backend; UI tests are `npm run test:engine-routing` and `npm run test:auth`.
