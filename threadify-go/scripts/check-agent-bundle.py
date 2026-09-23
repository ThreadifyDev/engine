#!/usr/bin/env python3
"""Reject incomplete agent runtime inputs before producing an Engine release."""
import hashlib
import json
from pathlib import Path
import tarfile

ROOT = Path(__file__).resolve().parents[1]
PLATFORMS = {"linux/amd64", "darwin/arm64", "windows/amd64"}


def check(root=ROOT):
    assets = root / "internal/agentbundle/assets"
    manifest = json.loads((assets / "runtimes.json").read_text())
    if set(manifest) != PLATFORMS:
        raise ValueError("agent release requires pinned runtimes for Linux AMD64, macOS ARM64 and Windows AMD64")
    for platform, entry in manifest.items():
        name = f"runtime-{platform.replace('/', '-')}.tar.gz"
        python = "python/python.exe" if platform.startswith("windows/") else "python/bin/python3.12"
        if entry.get("archive") != name or entry.get("python") != python:
            raise ValueError(f"invalid runtime layout: {platform}")
        hasher = hashlib.sha256()
        with (root / "agent-release" / name).open("rb") as stream:
            for chunk in iter(lambda: stream.read(1024 * 1024), b""):
                hasher.update(chunk)
        if hasher.hexdigest() != entry.get("sha256"):
            raise ValueError(f"runtime archive differs from embedded checksum: {platform}")
    with tarfile.open(assets / "agent.tar.gz", "r:gz") as archive:
        files = {member.name.removeprefix("./") for member in archive.getmembers() if member.isfile()}
        if not {"harnest-manifest.json", "launch.py"}.issubset(files):
            raise ValueError("compiled agent is missing required entry points")
    print("Agent artifact and all three pinned runtime archives verified")


if __name__ == "__main__":
    try:
        check()
    except (OSError, ValueError, tarfile.TarError) as error:
        raise SystemExit(f"Agent release inputs incomplete: {error}. Run the native runtime matrix and stage-engine-agent.py first.") from error
