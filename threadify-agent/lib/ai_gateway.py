"""Load the Engine's local AI settings and own one isolated model transport."""
from __future__ import annotations

import os
import ipaddress
import re
import ssl
from dataclasses import dataclass, field
from pathlib import Path
from urllib.parse import urlsplit

import httpx
import yaml
from harnest.model import LiteLLMLifecycle, LiteLLMModel
from openai import AsyncOpenAI


@dataclass(frozen=True)
class GatewaySettings:
    base_url: str
    model: str
    api_key: str = field(default="", repr=False)
    tls: ssl.SSLContext = field(default_factory=ssl.create_default_context, repr=False)
    installation_id: str = field(default="", repr=False)


def _mapping(value, allowed: set[str], name: str) -> dict:
    """Reject misspelled gateway options rather than silently losing safeguards."""
    if not isinstance(value, dict) or set(value) - allowed:
        raise ValueError(f"{name} must be a mapping containing only supported settings")
    return value


def _text(value, name: str) -> str:
    if not isinstance(value, str):
        raise ValueError(f"{name} must be a string")
    if value.startswith("$"):
        variable, _, default = value[1:].partition(":")
        value = os.environ.get(variable) or default
    return value.strip()


def _tls_context(options: dict, directory: Path) -> ssl.SSLContext:
    """Resolve certificate files against the YAML, retaining hostname verification."""
    options = _mapping(options, {"ca_file", "cert_file", "key_file"}, "ai.gateway.tls")
    paths = {name: _text(options.get(name, ""), f"ai.gateway.tls.{name}")
             for name in ("ca_file", "cert_file", "key_file")}
    if bool(paths["cert_file"]) != bool(paths["key_file"]):
        raise ValueError("ai.gateway.tls requires both cert_file and key_file")
    paths = {name: str(directory / value) if value else None for name, value in paths.items()}
    try:
        context = ssl.create_default_context(cafile=paths["ca_file"])
        if paths["cert_file"]:
            context.load_cert_chain(paths["cert_file"], paths["key_file"], password="")
        return context
    except (OSError, ssl.SSLError):
        raise ValueError("ai.gateway.tls certificate files are unreadable or invalid") from None


def load_gateway(path: str | Path) -> GatewaySettings:
    """Read only AI configuration; never expose other Engine settings or secrets."""
    path = Path(path).expanduser().resolve()
    try:
        document = yaml.safe_load(path.read_text())
    except (OSError, yaml.YAMLError):
        raise ValueError("Unable to read Threadify AI configuration YAML") from None
    if not isinstance(document, dict) or "ai" not in document:
        raise ValueError("Threadify configuration requires an ai section")
    ai = _mapping(document["ai"], {"enabled", "gateway", "agent"}, "ai")
    enabled = ai.get("enabled", True)
    if type(enabled) is not bool:
        raise ValueError("ai.enabled must be a boolean")
    if not enabled:
        raise ValueError("Threadify AI is disabled by configuration")
    gateway = _mapping(ai.get("gateway"), {"base_url", "model", "api_key_env", "tls", "auth"}, "ai.gateway")
    base_url = _text(gateway.get("base_url", ""), "ai.gateway.base_url")
    model = _text(gateway.get("model", ""), "ai.gateway.model")
    try:
        parsed = urlsplit(base_url)
        valid = (parsed.scheme in {"http", "https"} and parsed.hostname and
                 parsed.username is None and parsed.password is None and not parsed.query and not parsed.fragment)
        parsed.port  # Validate malformed ports before the first model request.
    except ValueError:
        valid = False
    if not valid or any(c.isspace() for c in base_url):
        raise ValueError("ai.gateway.base_url must be an HTTP(S) URL without credentials, query or fragment")
    if not model or any(c.isspace() for c in model):
        raise ValueError("ai.gateway.model must be a non-empty model identifier")
    tls = gateway.get("tls", {})
    if tls and parsed.scheme != "https":
        raise ValueError("ai.gateway.tls requires an HTTPS gateway")
    key_name = gateway.get("api_key_env", "")
    if not isinstance(key_name, str) or (key_name and not re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]*", key_name)):
        raise ValueError("ai.gateway.api_key_env must name an environment variable")
    key = os.environ.get(key_name, "") if key_name else ""
    if key_name and not key.strip():
        raise ValueError("The environment variable named by ai.gateway.api_key_env is empty or unset")
    installation = ""
    auth = gateway.get("auth", "custom")
    if auth not in ("custom", "threadify_license"):
        raise ValueError("ai.gateway.auth must be custom or threadify_license")
    if auth == "threadify_license":
        if key_name:
            raise ValueError("threadify_license auth cannot be combined with api_key_env")
        key, installation = _license_credentials(document, parsed)
    return GatewaySettings(base_url.rstrip("/"), model, key, _tls_context(tls, path.parent), installation)


def _license_credentials(document: dict, endpoint) -> tuple[str, str]:
    """Reuse the server's license only when hosted authentication is explicitly selected."""
    try:
        loopback = ipaddress.ip_address(endpoint.hostname).is_loopback
    except ValueError:
        loopback = endpoint.hostname == "localhost"
    if endpoint.scheme != "https" and not loopback:
        raise ValueError("threadify_license auth requires HTTPS except on loopback")
    registry = document.get("registry", {})
    if not isinstance(registry, dict):
        raise ValueError("registry must be a mapping for threadify_license auth")
    key = _text(registry.get("license_key", ""), "registry.license_key") or os.environ.get("THREADIFY_LICENSE_KEY", "").strip()
    if not key:
        raise ValueError("threadify_license auth requires the Engine's existing license")
    installation = _text(registry.get("installation_id", ""), "registry.installation_id") or os.environ.get("THREADIFY_INSTALLATION_ID", "").strip()
    return key, installation


class GatewayLifecycle(LiteLLMLifecycle):
    """Own TLS and credentials per model, without modifying global SDK settings."""

    def __init__(self, settings: GatewaySettings):
        self.settings = settings

    async def create_transport(self, context):
        async def remove_unused_auth(request: httpx.Request):
            # The SDK requires a key value; an unauthenticated/mTLS-only gateway
            # should receive neither a placeholder bearer nor ambient credentials.
            if not self.settings.api_key:
                request.headers.pop("Authorization", None)
            if self.settings.installation_id:
                request.headers["X-Threadify-Installation-ID"] = self.settings.installation_id

        http = httpx.AsyncClient(verify=self.settings.tls, trust_env=False,
                                follow_redirects=False, timeout=120,
                                event_hooks={"request": [remove_unused_auth]})
        return AsyncOpenAI(base_url=self.settings.base_url,
                           api_key=self.settings.api_key or "unused",
                           organization="", project="", max_retries=0,
                           http_client=http)

    async def close(self, context):
        if context.transport is not None:
            await context.transport.close()


def configured_model() -> LiteLLMModel:
    """An explicit YAML path takes precedence over legacy Ollama environment setup."""
    path = os.environ.get("THREADIFY_CONFIG_PATH")
    if path is None:
        return LiteLLMModel(model=os.getenv("LITELLM_MODEL", "ollama_chat/qwen3.5:cloud"))
    if not path.strip():
        raise ValueError("THREADIFY_CONFIG_PATH must name the Engine configuration YAML")
    settings = load_gateway(path)
    return LiteLLMModel(model=f"openai/{settings.model}", api_base=settings.base_url,
                        api_key=settings.api_key or "unused", lifecycle=GatewayLifecycle(settings))
