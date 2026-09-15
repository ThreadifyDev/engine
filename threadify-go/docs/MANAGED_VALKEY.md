# Managed and shared Valkey

Linux/macOS releases include Valkey 8.1.10 in `libexec/valkey-server`, beside
`threadify`. With no `redis` section, the Engine starts that executable and owns
its lifecycle. It listens on `127.0.0.1:6379` and stores data in `data/valkey`
beside the resolved Engine executable, regardless of the working directory.
Keep the complete release layout when moving the installation. PostgreSQL is
still external; NATS is embedded by default.

Windows builds use an external Valkey server, including one managed by a
Linux/macOS Engine. They do not include a native Valkey executable.

## One Engine owns Valkey; the others join it

Configure the owner explicitly when overriding its connection settings:

```yaml
redis:
  mode: managed
  bind: 0.0.0.0
  host: 127.0.0.1
  port: 6379
  password: "$VALKEY_PASSWORD"
  # Optional absolute path for a persistent volume:
  store_dir: /var/lib/threadify/valkey

nats:
  mode: external
  url: "$NATS_URL"
```

On each additional Engine:

```yaml
redis:
  mode: external
  host: 10.0.0.10 # Private address of the owner above
  port: 6379
  password: "$VALKEY_PASSWORD"
  db: 0

nats:
  mode: external
  url: "$NATS_URL"
```

All replicas of the installation must use the same PostgreSQL database, Valkey
endpoint and DB number, NATS/JetStream service, Registry license, and hash-chain
secrets. Omit the installation ID on all replicas to use the shared identity in
PostgreSQL, or supply the same explicit ID on every replica. Give each Engine its
own HTTP listener when running on one host.

`bind` is the owner's listening IP; `host` is the address that Engine uses to
connect. A non-loopback bind requires a password. Reach it only through a trusted
private network/VPN: the bundled listener uses TCP, without TLS. Expose only the
Engine's HTTP API publicly. For Engines on one machine, keep the loopback bind.

Valkey contains the shared live contract state. Existing Lua scripts perform
atomic transitions and permission claims there, so a second Engine sees those
changes before PostgreSQL archival completes. There is no new replica-local copy
of that state. Shared NATS is also necessary for billing coordination and
background work; separate default embedded brokers do not form one shared broker.

## Ownership and recovery

- Only `managed` starts/stops a child. `external` never starts a replacement,
  clears data, or stops the shared server when that Engine exits.
- Startup locks the store and verifies that the responding Valkey process is the
  child it launched. An occupied port or a second store owner is an error.
- Readiness waits for authenticated access and persistence loading. Missing
  executables, inaccessible storage, bad credentials and startup timeouts fail
  startup. The default startup deadline is 120 seconds; large datasets can set
  `redis.startup_timeout_seconds` explicitly.
- Managed Valkey uses AOF, `appendfsync always`, no eviction, and rejects truncated
  AOF logs. Acknowledged writes are fsynced before responses, at a latency cost
  that depends on storage. See [Valkey persistence](https://valkey.io/topics/persistence/).
- Shutdown drains Engine work before terminating its child. If the child dies,
  Engine health fails; it is not automatically replaced with an empty server.
- The owner host/process is still a failure dependency. Extra Engines do not
  provide Valkey failover. Restart the owner with the same persistent store.
  Stopping that owner makes Valkey unavailable to joining Engines until restart.
- If the owner is forcibly killed, its child may remain running and retain the
  store lock. Stop that orphan deliberately before restarting the owner. Never
  delete the lock file while a process is using the store.
- Back up the entire Valkey store, including its AOF directory and
  `.threadify-managed` marker, alongside JetStream and PostgreSQL. A marked store
  with a missing AOF manifest, an unrecognized nonempty store, or a damaged AOF
  requires explicit recovery; startup does not erase or automatically repair it.
  A startup interrupted before readiness may also require recovery of that store.

## Existing deployments

An existing `redis.host` or `redis.port` without `redis.mode` continues to select
`external`. Upgrading the installer preserves existing configuration. Set
`mode: managed` explicitly to opt into ownership when specifying a host or port.
There is **no automatic migration** of an existing external Valkey dataset:
changing to a fresh managed directory would lose its live state. Keep using the
existing server until you have planned a backed-up, quiesced migration.

For Docker, `/data/valkey` and `/data/jetstream` reside on the persistent `/data`
volume. Mount it durably and keep it writable by UID 65532. Storage can be
overridden with `REDIS_STORE_DIR` and `NATS_STORE_DIR` or YAML absolute paths.

For source builds on macOS, run `sh scripts/build-valkey.sh bin/libexec` after
building `bin/threadify`. The release workflow builds Linux in Alpine with musl
static linking and macOS on a native Apple Silicon runner. The script pins and
verifies the source archive SHA-256 and includes upstream/dependency notices.
For development only, `redis.binary_path` can point at an explicitly installed
Valkey server. Relative binary/store paths are resolved beside the Engine binary.

## Tests

`THREADIFY_TEST_VALKEY_BINARY=/absolute/path/to/valkey-server go test -race
./internal/managedvalkey` exercises real Lua claims through two independent
clients, process crash and graceful restart, duplicate owners, port collisions,
authentication failure, corruption, missing persistence and readiness timeout.

The compiled Engine tests accept `THREADIFY_SMOKE_BINARY`, a disposable
`THREADIFY_SMOKE_POSTGRES_URL`, and `THREADIFY_SMOKE_MANAGED_VALKEY_BINARY`.
Run `TestStandaloneBinaryPersistenceAndRestart` for default embedded NATS;
`TestTwoEnginesShareManagedValkey` starts two compiled Engines behind a test
proxy, sharing one managed Valkey and a disposable shared JetStream server.
Each run needs a fresh PostgreSQL database. Both exercise contracts, SDK waits,
unhappy paths and restart persistence. `THREADIFY_SMOKE_SDK_DIR` optionally
selects an isolated SDK checkout.
