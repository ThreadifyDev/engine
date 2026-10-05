# Browser SDK access

The JavaScript SDK package exports `@threadify/sdk/browser`. The browser uses
native WebSocket and a short-lived Engine grant; it never receives a service
API key.

Set `THREADIFY_BROWSER_ORIGINS` to the exact allowed application origins,
separated by commas. HTTPS is required except for loopback development.
Same-origin Engine pages are allowed automatically. Restart the Engine after
changing the environment variable.

An authenticated application backend creates a grant with
`POST /v1/browser-tokens`, sending its service key in `X-API-Key` and a JSON
body such as:

```json
{
  "origin": "http://localhost:5173",
  "actions": ["startThread", "recordThreadEvent", "recordBrowserAction", "waitFor"],
  "thread_keys": ["order:123"],
  "thread_ids": [],
  "ttl_seconds": 300
}
```

The response contains `token` and `expires_at`. The backend must decide which
thread keys and actions the signed-in user may access. Grants last at most five
minutes. `startThread` requires a listed thread key, including when resuming a
thread. Other thread actions require a listed thread ID or a thread opened on
the same WebSocket connection. Revoking the source service key invalidates its
grants. Browser-origin requests cannot mint grants directly.

Allowed actions are `startThread`, `recordThreadEvent`, `recordBrowserAction`, `waitFor`, `addRefs`,
`closeThread`, and `threadEnd`. Heartbeat, wait cancellation, connection close,
and grant renewal are available for the connected session. Global
notifications and direct GraphQL reads are outside browser grant scope.

For a local smoke test, start the Engine with
`THREADIFY_BROWSER_ORIGINS=http://localhost:5173`, create a grant using a
service key from your backend, then connect a page at that origin with
`ThreadifyBrowser.connect({ engineUrl, getAccessToken })`. Use
`connection.thread('order:123')` and record a step. Test a different thread
key and an unlisted action; both should receive a grant denial. The service
key must stay in your backend's environment.

`recordBrowserAction` stores bounded action evidence in the thread activity log.
For contracted threads the Engine marks an exact or classifier matched action
as a step candidate; otherwise it records a substep under the latest completed
step when one exists. Free-form threads use the `free_form` classification.
The browser SDK can map a click directly to `recordThreadEvent`.

Administrators can open **Input config** from the relevant contract version page.
These mappings apply to OTel spans and auto-captured browser actions. Direct
Threadify SDK events bypass them.
Enter one `action=contract_step` pair per line, or use
`action_a,action_b=contract_step` to map several actions to the same step.
Each action becomes one rule with a captured action
name (exact, one trailing `*` prefix, or `regex:` expression), a contract name, a version, and a step
name. The Engine validates the contract version and step when saving. On future
captured actions, a matching rule records the step on
the same thread through the normal step path, including context, access, and
contract validation. The thread's pinned contract version is checked again at
record time. Exact action names win over prefixes; the longest prefix wins
among wildcard rules. Regex is considered next, with the first matching regex
in authored order winning. A failed mapping remains in the activity log with
`mapping_rejected` and a reason. Existing activities are not turned into
completed steps when a rule is added. Grant only the actions a page needs.

Regex mappings use the same Go/RE2 syntax as general trace filtering, including
`(?i)` for case-insensitive matching. Use one regex mapping per line:

```text
regex:(?i)^POST /checkout=order_placed
regex:(?i)^checkout_(clicked|confirmed)$=order_placed
```

The final `=` separates the target step. Commas and earlier `=` characters belong
to the expression, so repetitions such as `{1,3}` and literal query values work.
Regex searches anywhere unless anchored with `^` or `$`. Invalid regex,
lookarounds, and backreferences are rejected. Mapping names remain limited to
128 UTF-8 bytes. Plain comma-separated action lists keep their existing syntax.

When a new contract version is published, the Engine copies action links from
the previous latest version if their target steps still exist. Links to removed
steps are skipped and the publish response reports the copied and skipped counts.

The management API is `GET` or `PUT /v1/engine/browser-action-mappings` with
an administrator session or user API key. A save uses the latest `revision`:

```json
{
  "revision": "",
  "rules": [
    { "action": "checkout_confirmed", "contract": "order_flow", "version": 1, "step": "order_placed" }
  ]
}
```
