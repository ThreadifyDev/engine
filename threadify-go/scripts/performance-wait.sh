#!/usr/bin/env bash
# Local SDK/Engine benchmark. Uses new disposable containers, never developer DBs.
set -euo pipefail
engine_dir="$(cd "$(dirname "$0")/.." && pwd)"
output="${1:-$engine_dir/docs/performance/wait-$(date +%Y%m%d-%H%M%S).json}"
mkdir -p "$(dirname "$output")"
output="$(cd "$(dirname "$output")" && pwd)/$(basename "$output")"
work="$(mktemp -d "${TMPDIR:-/tmp}/threadify-perf.XXXXXX")"
pg_name="codex-threadify-perf-pg-$$"
valkey_name="codex-threadify-perf-valkey-$$"
cleanup() {
  docker rm -f -v "$pg_name" "$valkey_name" >/dev/null 2>&1 || true
  rm -rf "$work"
}
trap cleanup EXIT
cd "$engine_dir"
go build -o "$work/threadify" ./cmd/server
docker run -d --name "$pg_name" -e POSTGRES_PASSWORD=integration-only -e POSTGRES_DB=threadify_test -p 127.0.0.1::5432 postgres:16-alpine >/dev/null
docker run -d --name "$valkey_name" -p 127.0.0.1::6379 valkey/valkey:9-alpine >/dev/null
for attempt in $(seq 1 60); do
  if docker exec "$pg_name" pg_isready -U postgres >/dev/null 2>&1; then break; fi
  sleep 1
done
pg_port="$(docker port "$pg_name" 5432/tcp)"
valkey_addr="$(docker port "$valkey_name" 6379/tcp)"
THREADIFY_SMOKE_BINARY="$work/threadify" \
THREADIFY_SMOKE_POSTGRES_URL="postgres://postgres:integration-only@$pg_port/threadify_test?sslmode=disable" \
THREADIFY_SMOKE_VALKEY_ADDR="$valkey_addr" \
THREADIFY_SMOKE_REGISTRY_URL='' \
THREADIFY_PERF_OUTPUT="$output" \
go test ./cmd/server -run '^TestStandaloneBinaryPersistenceAndRestart$' -count=1 -v -timeout=15m | tee "$output.log"
