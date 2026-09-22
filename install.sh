#!/bin/sh
# Download a verified Threadify Engine release. Requires curl, tar/unzip and a SHA-256 tool.
set -eu

fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
usage() {
  cat <<'EOF'
Install Threadify, bundled Valkey (Linux/macOS), and missing configuration templates.

Usage: sh install.sh [--version VERSION] [--bin-dir DIR] [--config-dir DIR]

  --version VERSION  Stable release (e.g. v1.2.3 or 1.2.3); default: latest
  --bin-dir DIR      Binary directory; default: $HOME/.local/bin
  --config-dir DIR   Configuration directory; default: $XDG_CONFIG_HOME/threadify
                    or $HOME/.config/threadify
  --help            Show this help

Supports Apple Silicon macOS, x86-64 Linux and x86-64 Windows (Git Bash).
Existing config files are preserved. Does not start services or edit shell profiles.
EOF
}

version=latest
bin_dir=${HOME:?HOME is required}/.local/bin
config_dir=${XDG_CONFIG_HOME:-$HOME/.config}/threadify
while [ "$#" -gt 0 ]; do
  case "$1" in
    --version|--bin-dir|--config-dir)
      [ "$#" -ge 2 ] && [ -n "$2" ] || fail "$1 requires a value"
      case "$1" in
        --version) version=$2 ;;
        --bin-dir) bin_dir=$2 ;;
        --config-dir) config_dir=$2 ;;
      esac
      shift 2 ;;
    --help|-h) usage; exit 0 ;;
    *) fail "Unknown option: $1 (use --help)" ;;
  esac
done

command -v curl >/dev/null 2>&1 || fail 'curl is required'
os=$(uname -s)
arch=$(uname -m)
case "$os:$arch" in
  Darwin:arm64) target=darwin_arm64; extension=tar.gz; binary=threadify ;;
  Linux:x86_64|Linux:amd64) target=linux_amd64; extension=tar.gz; binary=threadify ;;
  MINGW*:x86_64|MSYS*:x86_64|CYGWIN*:x86_64) target=windows_amd64; extension=zip; binary=threadify.exe ;;
  *) fail "No release binary for $os/$arch. Available: macOS arm64, Linux amd64, Windows amd64." ;;
esac
if [ "$extension" = zip ]; then
  command -v unzip >/dev/null 2>&1 || fail 'unzip is required on Windows (Git Bash)'
else
  command -v tar >/dev/null 2>&1 || fail 'tar is required'
fi
if command -v sha256sum >/dev/null 2>&1; then
  hash_tool=sha256sum
elif command -v shasum >/dev/null 2>&1; then
  hash_tool=shasum
else
  fail 'sha256sum or shasum is required to verify the download'
fi

release_root=https://github.com/ThreadifyDev/engine/releases
if [ "$version" = latest ]; then
  # Follow GitHub's canonical release redirect; no JSON parser or GitHub token is needed.
  resolved=$(curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --silent --show-error --location --retry 3 --connect-timeout 15 --max-time 120 --output /dev/null --write-out '%{url_effective}' "$release_root/latest") || fail 'Could not resolve the latest release'
  case "$resolved" in
    "$release_root"/tag/v*) version=${resolved##*/} ;;
    *) fail 'The latest release did not resolve to an Engine version; use --version' ;;
  esac
fi
version=${version#v}
printf '%s\n' "$version" | grep -Eq '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' || fail 'Version must be a stable release such as v1.2.3'
archive=threadify_${version}_${target}.${extension}
download_root=$release_root/download/v$version

work=$(mktemp -d "${TMPDIR:-/tmp}/threadify-install.XXXXXX") || fail 'Cannot create temporary directory'
staged=
cleanup() {
  if [ -n "$staged" ]; then rm -f "$staged"; fi
  rm -rf "$work"
}
trap cleanup EXIT
trap 'exit 1' HUP INT TERM
download() {
  curl --proto '=https' --proto-redir '=https' --tlsv1.2 --fail --silent --show-error --location --retry 3 --connect-timeout 15 --max-time 600 --output "$2" "$1" || fail "Download failed: $1"
}

printf 'Downloading Threadify v%s (%s)…\n' "$version" "$target"
download "$download_root/$archive" "$work/$archive"
download "$download_root/checksums.txt" "$work/checksums.txt"
expected=$(awk -v name="$archive" '$2 == name || $2 == "*" name {print $1}' "$work/checksums.txt")
printf '%s\n' "$expected" | grep -Eq '^[0-9a-fA-F]{64}$' || fail 'Missing or ambiguous archive checksum'
[ "$(printf '%s\n' "$expected" | wc -l | tr -d ' ')" = 1 ] || fail 'Duplicate archive checksum'
if [ "$hash_tool" = sha256sum ]; then
  actual=$(sha256sum "$work/$archive" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$work/$archive" | awk '{print $1}')
fi
[ "$actual" = "$(printf '%s' "$expected" | tr 'A-F' 'a-f')" ] || fail 'Checksum verification failed; nothing was installed'

mkdir "$work/extracted"
bundled=false
if [ "$extension" = tar.gz ]; then
  tar -tzf "$work/$archive" > "$work/members" || fail 'Invalid release archive'
  if grep -Fxq 'libexec/valkey-server' "$work/members"; then bundled=true; fi
fi
# Extract only the expected executable and reviewed templates, never arbitrary archive paths.
if [ "$extension" = zip ]; then
  unzip -q "$work/$archive" "$binary" config/config.yaml -d "$work/extracted" || fail 'Could not extract release files'
else
  tar -xzf "$work/$archive" -C "$work/extracted" "$binary" config/config.yaml || fail 'Could not extract release files'
fi
if [ "$bundled" = true ]; then
  tar -xzf "$work/$archive" -C "$work/extracted" libexec/valkey-server libexec/VALKEY-LICENSES.txt || fail 'Could not extract bundled Valkey'
  for file in libexec/valkey-server libexec/VALKEY-LICENSES.txt; do
    [ -f "$work/extracted/$file" ] && [ ! -L "$work/extracted/$file" ] || fail "Invalid release file: $file"
  done
fi
for file in "$binary" config/config.yaml; do
  [ -f "$work/extracted/$file" ] && [ ! -L "$work/extracted/$file" ] || fail "Invalid release file: $file"
done

# Stage the binary on the destination filesystem so replacement is atomic.
mkdir -p "$bin_dir" "$config_dir" || fail 'Cannot create installation directories; choose writable --bin-dir and --config-dir'
bin_dir=$(cd "$bin_dir" && pwd)
config_dir=$(cd "$config_dir" && pwd)
[ ! -d "$bin_dir/$binary" ] || fail "Binary destination is a directory: $bin_dir/$binary"
for file in config.yaml; do
  if [ -e "$config_dir/$file" ] || [ -L "$config_dir/$file" ]; then
    printf 'Preserving %s\n' "$config_dir/$file"
  else
    # noclobber also protects a config created by another installer after the existence check.
    (umask 077; set -C; cat "$work/extracted/config/$file" > "$config_dir/$file") || fail "Could not create $config_dir/$file"
  fi
done
if [ "$bundled" = true ]; then
  mkdir -p "$bin_dir/libexec"
  for file in valkey-server VALKEY-LICENSES.txt; do
    [ ! -d "$bin_dir/libexec/$file" ] || fail "Bundled destination is a directory: $file"
    staged=$(mktemp "$bin_dir/libexec/.threadify-install.XXXXXX")
    cp "$work/extracted/libexec/$file" "$staged"
    case "$file" in valkey-server) chmod 755 "$staged" ;; *) chmod 644 "$staged" ;; esac
    mv -f "$staged" "$bin_dir/libexec/$file"
    staged=
  done
fi
staged=$(mktemp "$bin_dir/.threadify-install.XXXXXX") || fail 'Cannot stage the binary'
cp "$work/extracted/$binary" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$bin_dir/$binary" || fail 'Cannot replace the binary; stop a running Windows Engine before upgrading'
staged=
printf '\nInstalled Threadify v%s at %s\n' "$version" "$bin_dir/$binary"
printf 'Configure %s and supply your Registry license and PostgreSQL settings.\n' "$config_dir/config.yaml"
if [ "$bundled" = true ]; then
  printf 'Valkey starts automatically; keep libexec beside the Engine binary.\n'
else
  printf 'This archive requires an external Valkey server.\n'
fi
printf 'Start with: "%s" --config "%s"\n' "$bin_dir/$binary" "$config_dir/config.yaml"
case :${PATH:-}: in
  *:"$bin_dir":*) ;;
  *) printf 'Add %s to PATH to run threadify from any directory.\n' "$bin_dir" ;;
esac
