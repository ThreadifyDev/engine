#!/usr/bin/env python3
"""Start the analyst with the key matching the dedicated local Threadify instance."""
import json
import os
from pathlib import Path
import subprocess
import sys

agent = Path(__file__).resolve().parent
state = agent.parents[1] / ".threadify-local"
environment = os.environ.copy()
default_credentials = state / "analyst-credentials.json"
if not default_credentials.exists():
    default_credentials = state / "credentials.json"
credentials = json.loads(Path(environment.get("THREADIFY_CREDENTIALS_FILE", str(default_credentials))).read_text())
environment["THREADIFY_API_KEY"] = credentials["api_key"]
environment["THREADIFY_GRAPHQL_URL"] = credentials["engine_url"] + "/graphql"
# OllamaModel uses the native /api/chat route, not the OpenAI-compatible /v1 API.
if environment.get("OLLAMA_BASE_URL"):
    environment["OLLAMA_BASE_URL"] = environment["OLLAMA_BASE_URL"].rstrip("/").removesuffix("/v1")
environment.setdefault("OLLAMA_MODEL", environment.get("OLLAMA_ANSWER_MODEL", "qwen3.5:cloud"))
environment.setdefault("LITELLM_LOCAL_MODEL_COST_MAP", "True")
arguments = sys.argv[1:] or ["serve", "--host", "127.0.0.1", "--port", "8121"]
if arguments[0] not in {"serve", "test", "compile"}:
    raise SystemExit("Usage: run_local.py [serve|test|compile] [options]")
runtime_python = environment.get("HARNEST_PYTHON")
launcher = [runtime_python, "-m", "harnest.cli"] if runtime_python else ["harnest"]
command = [*launcher, arguments[0], str(agent), *arguments[1:]]
if runtime_python and arguments[0] == "serve":
    output = agent / ".harnest/local-runtime"
    subprocess.check_call([*launcher, "compile", str(agent), "--output", str(output)], env=environment)
    command = [runtime_python, str(output / "harnest-agent"), *arguments]
if arguments[0] == "compile" and "--output" not in arguments and "-o" not in arguments:
    command.extend(["--output", str(agent / ".harnest/build")])
raise SystemExit(subprocess.call(command, env=environment))
