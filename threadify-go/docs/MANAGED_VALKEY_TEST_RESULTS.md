# Managed Valkey verification — 2026-09-15

Implemented and tested locally; these results do not represent a published release.

## Runtime and integration

- Full Engine Go suite: passed after the final implementation changes.
- Managed Valkey and configuration race tests: passed on macOS with real Valkey 8.1.10.
- Managed runtime tests: passed on Linux amd64, under the release container's
  non-root user, using the static bundled Valkey executable.
- Compiled standalone Engine with embedded NATS, managed Valkey, disposable
  PostgreSQL and signed Registry fixture: passed SDK contract creation, waits,
  rule violations, thread persistence and restart recovery.
- Two compiled Engines sharing one owner-managed Valkey, one JetStream server and
  PostgreSQL: passed the same SDK checks. An explicit extra check recorded an
  approval through one Engine, claimed it through the other, and confirmed that
  the first Engine could not reuse it. The outstanding grant survived restart.
- SDK tests used committed SDK revision `1eca533` in an isolated checkout because
  unrelated SDK working-tree changes had removed its package/entry-point files.
- The SDK run includes 20 permission timing samples, but this was a correctness
  smoke test on a busy development machine, not a controlled performance comparison.

## Unhappy paths

Verified duplicate store owners, occupied ports (without attaching to the wrong
server), missing executable, invalid/non-authenticated network bind, wrong
password, startup cancellation/timeout, missing AOF manifest, truncated AOF,
child-process crash and graceful restart. Thirty-two competing claims through
two independent clients yielded exactly one grant.

Forcibly killing an owner left its child holding the store lock. Starting another
owner failed until that orphan was stopped. The test then restarted the same store.

The two-Engine test found a pre-existing shutdown ordering problem: a dependent
archival batch could be delivered to an Engine whose peer had not yet written its
thread metadata. Shutdown now returns that batch to JetStream for replay without
acknowledging it or reporting the temporary dependency as a fatal flush failure.
Requeue failures still fail shutdown. Archiver race/regression tests passed, and
the final two-Engine SDK/restart test passed with the fix.

## Distribution

- Built the pinned source on native macOS and in Linux amd64 Alpine with static linking.
- Built Linux amd64, macOS arm64 and Windows amd64 Engine archives with GoReleaser.
- Verified archive checksums, executable permissions, architecture and bundled notices.
- Added a pre-release check that rejects missing/wrong-platform Valkey bundles.
- Built the non-root release container and executed both shipped binaries inside it.
- Nine offline installer tests passed, including bundled executable/notices and
  preservation of existing configuration.
- GoReleaser configuration, workflow YAML and whitespace checks passed.

The owner remains a single Valkey failure dependency. There is no automatic
failover or external-data migration. Windows Engines join an external/shared
server. Configuration and recovery instructions are in [MANAGED_VALKEY.md](MANAGED_VALKEY.md).
