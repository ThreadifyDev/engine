#!/usr/bin/env python3
"""Build the portable runtime and official compiled production agent artifact."""
import argparse
import gzip
import hashlib
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
PYTHON = "3.12.14"


def archive(source, output):
    """Write portable regular-file archives with stable metadata."""
    with output.open("wb") as raw, gzip.GzipFile(filename="", fileobj=raw, mode="wb", mtime=0) as gz:
        with tarfile.open(fileobj=gz, mode="w") as tar:
            for path in sorted(source.rglob("*")):
                if "__pycache__" in path.parts or path.suffix == ".pyc":
                    continue
                info = tar.gettarinfo(str(path), str(path.relative_to(source)))
                info.uid = info.gid = info.mtime = 0
                info.uname = info.gname = ""
                if path.is_file():
                    # Dereference interpreter aliases: installer rejects archive links.
                    info.type = tarfile.REGTYPE
                    info.linkname = ""
                    info.size = path.stat().st_size
                    with path.open("rb") as stream:
                        tar.addfile(info, stream)
                elif path.is_dir():
                    info.type = tarfile.DIRTYPE
                    info.linkname = ""
                    tar.addfile(info)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--compile-only", action="store_true")
    parser.add_argument("--python", help="Existing build interpreter for --compile-only")
    args = parser.parse_args()
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=True)
    target = {"Darwin": "darwin", "Linux": "linux", "Windows": "windows"}[platform.system()]
    arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "AMD64": "amd64"}[platform.machine()]
    wheel = next((ROOT / "runtime/vendor").glob("*.whl"))
    wheel_hash = hashlib.sha256(wheel.read_bytes()).hexdigest()
    with tempfile.TemporaryDirectory(prefix="threadify-agent-build-") as temporary:
        work = Path(temporary)
        runtime = work / "runtime"
        packages = runtime / "packages"
        packages.mkdir(parents=True)
        if args.compile_only:
            python = args.python or shutil.which("python3")
            with zipfile.ZipFile(wheel) as z:
                z.extractall(packages)
        else:
            env = dict(os.environ, UV_PYTHON_INSTALL_DIR=str(work / "python-install"))
            subprocess.run(["uv", "python", "install", PYTHON], env=env, check=True)
            installed = subprocess.check_output(["uv", "python", "find", "--managed-python", PYTHON], env=env, text=True).strip()
            home = Path(installed).resolve().parent
            if target != "windows":
                home = home.parent
            shutil.copytree(home, runtime / "python", symlinks=False)
            executable = "python/python.exe" if target == "windows" else "python/bin/python3.12"
            python = str(runtime / executable)
            lock = (ROOT / "harnest-runtime.lock").read_text()
            lock = re.sub(r"harnest @ __HARNEST_RELEASE_WHEEL_URI__ \\\n\s+--hash=sha256:[a-f0-9]+", f"harnest @ {wheel.as_uri()} \\\n    --hash=sha256:{wheel_hash}", lock)
            requirements = work / "requirements.txt"
            requirements.write_text(lock)
            subprocess.run(["uv", "pip", "sync", "--python", python, "--target", str(packages), "--require-hashes", str(requirements)], check=True)
            subprocess.run([python, "-I", str(ROOT / "runtime/smoke.py"), str(packages)], check=True)
            runtime_archive = output / f"runtime-{target}-{arch}.tar.gz"
            archive(runtime, runtime_archive)
            entry = {"sha256": hashlib.sha256(runtime_archive.read_bytes()).hexdigest(), "archive": runtime_archive.name, "python": executable}
            (output / f"manifest-{target}-{arch}.json").write_text(json.dumps({f"{target}/{arch}": entry}, indent=2) + "\n")
        source = work / "source"
        source.mkdir()
        # SDK extensions are examples; production tools authenticate as the caller.
        for name in ("agent.py", "agent-card.yaml", "config.yaml", "instructions.md", "pyproject.toml", "harnest.lock", "harnest-runtime.lock", "lib", "lifecycle", "tools", "models", "skills", "subagents", "plugins", "mcp", "sandbox"):
            path = ROOT / name
            if path.is_dir():
                shutil.copytree(path, source / name, ignore=shutil.ignore_patterns("__pycache__", "*.pyc"))
            elif path.exists():
                shutil.copy2(path, source / name)
        compiled = work / "compiled"
        env = dict(os.environ, PYTHONPATH=str(packages), PYTHONDONTWRITEBYTECODE="1", LITELLM_LOCAL_MODEL_COST_MAP="True")
        subprocess.run(["harnest", "compile", str(source), "--output", str(compiled), "--python", str(python)], env=env, check=True)
        shutil.copy2(ROOT / "runtime/launch.py", compiled / "launch.py")
        archive(compiled, output / "agent.tar.gz")


if __name__ == "__main__":
    main()
