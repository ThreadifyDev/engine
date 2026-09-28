"""Run this sample against a disposable Engine with the configured real model.

Called by the Go binary fixture. Only synthetic fixture threads are accessible
through the supplied MCP endpoint; the user's running Engine is never used.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

url, key, thread_id, workflow_ref, mode, evidence_file = sys.argv[1:]
project = Path(__file__).resolve().parents[2]
private = json.loads((Path.home() / ".config/threadify-mcp-agent/environment.json").read_text())
env = os.environ.copy()
# The existing model credentials go only to their configured model provider.
# Deliberately do not load the existing Engine's URL or API key.
for name in ("OPENAI_API_KEY", "OPENAI_BASE_URL", "OPENAI_MODEL"):
    env[name] = private[name]
for name in list(env):
    if name.startswith("OTEL_"):
        del env[name]
env.update({
    "THREADIFY_MCP_URL": url + "/mcp",
    "THREADIFY_API_KEY": key,
    "THREADIFY_SMOKE_THREAD_ID": thread_id,
    "THREADIFY_LIVE_MODEL_TEST": "0",
    "THREADIFY_AGENT_CORRELATION_TEST": "1",
    "THREADIFY_AGENT_EVIDENCE_FILE": evidence_file,
    "HARNEST_OTEL_ENABLED": "true",
    "OTEL_TRACES_EXPORTER": "otlp",
    "OTEL_LOGS_EXPORTER": "none",
    "OTEL_EXPORTER_OTLP_TRACES_PROTOCOL": "http/protobuf",
    "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT": url + "/v1/traces" + ("?use_workflow_run_id=false" if mode == "trace" else ""),
    "OTEL_EXPORTER_OTLP_TRACES_HEADERS": "X-API-Key=" + key,
    # This process is dedicated to one synthetic workflow run.
    "OTEL_RESOURCE_ATTRIBUTES": "workflow.run_id=" + workflow_ref,
    "OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT": "NO_CONTENT",
    "ADK_CAPTURE_MESSAGE_CONTENT_IN_SPANS": "false",
})
with tempfile.TemporaryDirectory(prefix="threadify-harnest-correlation-") as temporary:
    target = Path(temporary) / "agent"
    shutil.copytree(project, target, ignore=shutil.ignore_patterns(".harnest", "__pycache__", ".pytest_cache", ".git"))
    command = [str(Path.home() / ".local/bin/harnest"), "test", str(target), "--smoke", "--python", sys.executable]
    result = subprocess.run(command, env=env, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=240)
    # Runtime logs may contain request headers on failures; redact fixture/provider keys.
    output = result.stdout.replace(key, "[fixture key]").replace(private["OPENAI_API_KEY"], "[model key]")
    Path(evidence_file + ".log").write_text(output)
    if result.returncode:
        print(output[-14000:])
        raise SystemExit(result.returncode)
    print(json.dumps({"mode": mode, "evidence_file": evidence_file, "passed": True}))
