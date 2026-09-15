#!/bin/sh
# Build the pinned server on the target platform. Linux uses musl static linking
# in the release workflow so the result also runs in the distroless image.
set -eu
output=${1:?usage: build-valkey.sh OUTPUT_DIRECTORY}
mkdir -p "$output"
output=$(cd "$output" && pwd)
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
curl --proto '=https' --tlsv1.2 --fail --location --retry 3 --output "$work/source.tar.gz" \
  https://codeload.github.com/valkey-io/valkey/tar.gz/refs/tags/8.1.10
expected=c74e50cd83f6d398a3dc570e04ac2fe538249585d021f76ee2449bbf9ebd04ed
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$work/source.tar.gz" | cut -d ' ' -f 1)
else
  actual=$(shasum -a 256 "$work/source.tar.gz" | cut -d ' ' -f 1)
fi
[ "$expected" = "$actual" ] || { echo 'Valkey source checksum mismatch' >&2; exit 1; }
tar -xzf "$work/source.tar.gz" -C "$work"
cd "$work/valkey-8.1.10"
case $(uname -s) in
  Linux) make -j 2 MALLOC=libc BUILD_TLS=no LDFLAGS=-static valkey-server ;;
  Darwin) MACOSX_DEPLOYMENT_TARGET=13.0 make -j 2 MALLOC=libc BUILD_TLS=no valkey-server ;;
  *) echo 'Bundled Valkey requires Linux or macOS' >&2; exit 1 ;;
esac
cp src/valkey-server "$output/valkey-server"
chmod 755 "$output/valkey-server"
# Preserve upstream and bundled dependency notices in the distributed artifact.
{
  printf 'Valkey 8.1.10: https://github.com/valkey-io/valkey/tree/8.1.10\n\n'
  for notice in COPYING deps/lua/COPYRIGHT deps/hdr_histogram/LICENSE.txt deps/hdr_histogram/COPYING.txt deps/fpconv/LICENSE.txt deps/hiredis/COPYING; do
    printf '\n--- %s ---\n' "$notice"
    cat "$notice"
  done
} > "$output/VALKEY-LICENSES.txt"
"$output/valkey-server" --version
