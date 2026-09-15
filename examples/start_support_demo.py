#!/usr/bin/env python3
"""Start both loopback Harnest playgrounds, without sending a model prompt."""
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import time
import urllib.request

root = Path(__file__).resolve().parents[1]
state = root / ".threadify-local"
state.mkdir(mode=0o700, exist_ok=True)
os.umask(0o077)
results = []
for name, folder, port in [("support", "threadify-mini-agent", 8120), ("analyst", "threadify-analyst-agent", 8121)]:
    with socket.socket() as probe:
        if probe.connect_ex(("127.0.0.1", port)) == 0:
            raise SystemExit(f"Port {port} is already occupied; use the existing server or stop its recorded process before relaunching.")
    with (state / f"{name}-server.log").open("ab") as log:
        child = subprocess.Popen([sys.executable, str(root / "examples" / folder / "run_local.py"), "serve", "--host", "127.0.0.1", "--port", str(port)], cwd=root, stdin=subprocess.DEVNULL, stdout=log, stderr=subprocess.STDOUT, start_new_session=True)
    (state / f"{name}-server.pid").write_text(str(child.pid) + "\n")
    deadline = time.monotonic() + 90
    while time.monotonic() < deadline:
        if child.poll() is not None:
            raise SystemExit(f"{name} server exited; see {state / (name + '-server.log')}")
        try:
            with urllib.request.urlopen(f"http://127.0.0.1:{port}/healthz", timeout=2) as response:
                if response.status == 200:
                    break
        except OSError:
            pass
        time.sleep(0.5)
    else:
        raise SystemExit(f"{name} server is still starting; see its log")
    result = {"agent": name, "pid": child.pid, "url": f"http://127.0.0.1:{port}/"}
    results.append(result)
    print(json.dumps(result), flush=True)
(state / "agent-servers.json").write_text(json.dumps(results, indent=2) + "\n")
