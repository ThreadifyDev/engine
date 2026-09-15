#!/usr/bin/env python3
"""Choose an engine tag from engine commits; never mutate the repository."""
import os
import re
import subprocess

# Keep these aligned with the engine workflow path filters. SDK/API/UI commits
# must neither create engine releases nor determine their semantic version bump.
PATHS = [
    "threadify-go", ":(exclude)threadify-go/api", ":(exclude)threadify-go/tests/api",
    ".github/workflows/engine-ci.yml", ".github/workflows/engine-release.yml",
    "install.sh", ".github/scripts/test_engine_installer.py",
    ".github/scripts/engine-version.py", ".github/scripts/test_engine_version.py",
]
STABLE = re.compile(r"v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$")


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def next_release():
    """Return the tag and previous engine tag, reusing tags on retried runs."""
    versions = sorted(
        (tuple(map(int, match.groups())), tag)
        for tag in git("tag", "--merged", "HEAD").splitlines()
        if (match := STABLE.fullmatch(tag))
    )
    previous = versions[-1][1] if versions else ""
    if previous and git("rev-list", "-n", "1", previous) == git("rev-parse", "HEAD"):
        return previous, versions[-2][1] if len(versions) > 1 else ""
    revision = f"{previous}..HEAD" if previous else "HEAD"
    messages = git("log", "--format=%B%x00", revision, "--", *PATHS)
    if not messages:
        return "", previous
    major, minor, patch = versions[-1][0] if versions else (0, 0, 0)
    if re.search(r"(?m)^[\w-]+(?:\([^\n]*\))?!:|^BREAKING[ -]CHANGE:", messages):
        major, minor, patch = major + 1, 0, 0
    elif re.search(r"(?m)^feat(?:\([^\n]*\))?:", messages):
        minor, patch = minor + 1, 0
    else:
        patch += 1
    return f"v{major}.{minor}.{patch}", previous


if __name__ == "__main__":
    tag, previous = next_release()
    print(f"Engine release: {tag or 'no engine changes'} (previous: {previous or 'none'})")
    if output := os.environ.get("GITHUB_OUTPUT"):
        with open(output, "a", encoding="utf-8") as stream:
            stream.write(f"tag={tag}\nprevious_tag={previous}\n")
