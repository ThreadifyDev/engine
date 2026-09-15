# Local management lifecycle checks

Run against an explicitly configured local Web API and Engine with a bearer
session that can manage disposable records:

```sh
THREADIFY_LIVE_BEARER_FILE=/absolute/path/to/token.txt \
THREADIFY_LIVE_API_URL=http://127.0.0.1:3003 \
THREADIFY_LIVE_ENGINE_URL=http://127.0.0.1:8083 \
python3 -m unittest discover -s threadify-go/tests/live -v
```

The six tests cover profile-type dry run/create/read/update/rename/archive,
service-account create/read/update/delete, API-key create/list/revoke, contract
preview/create/read/version update/delete, account/team reads, and unauthenticated
request rejection. Fixtures have unique names and are archived/deleted/revoked
using the normal API; existing user records are not edited. No email is sent and
no billing endpoint is exercised. Normal request metering still applies.

Tests skip unless the bearer-file environment variable is supplied. Both service
URLs must be loopback addresses. Bearer tokens and newly generated API keys are
never printed. The suite does not test signup, password recovery, invitation
acceptance, cross-account permissions, or browser form interactions; those need
the isolated integration suite and its fake identity/email providers.
