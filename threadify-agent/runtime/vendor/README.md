# Pinned runtime build input

`harnest-0.23.0-py3-none-any.whl` is the runtime wheel used by the Threadify
agent archive builder. It includes Python source and Apache-2.0 license metadata.

- Upstream: https://github.com/Usefused/harnest
- Base revision: `553638d5a12770c0bc9f777b4d10e18a73a19c05`
- Snapshot: local source on 2026-09-22, including the authenticated client-tool
  continuation fixes tested with Threadify. This is not an upstream release tag.
- SHA-256: `b3f7d6262e761b0df4fefaf8339e6cf3cd9c9fbca4525fc157b64479567ac909`
- Build command: `uv build --wheel /path/to/harnest --out-dir /tmp/wheels`

The committed wheel is the immutable build input; rebuilding an uncommitted
upstream checkout is not part of Engine CI. Update this wheel deliberately,
review its source changes, rebuild all platform runtime archives, and regenerate
the embedded manifest together. Third-party dependencies use the hash-pinned
agent runtime lock. Python is the pinned portable CPython 3.12.14 distribution
installed by uv.
