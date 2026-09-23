"""Configuration and real HTTP/TLS model transport regression checks."""
import asyncio
import ipaddress
import json
import ssl
import threading
from datetime import datetime, timedelta, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

import pytest
import litellm
import yaml
from cryptography import x509
from cryptography.hazmat.primitives import hashes, serialization
from cryptography.hazmat.primitives.asymmetric import rsa
from cryptography.x509.oid import ExtendedKeyUsageOID, NameOID
from harnest.lib.ai_gateway import configured_model, load_gateway


def write_config(tmp_path, gateway, **ai):
    path = tmp_path / 'engine.yaml'
    path.write_text(yaml.safe_dump({'server': {'port': 8081}, 'ai': {'gateway': gateway, **ai}}))
    return path


def test_yaml_wins_over_ambient_provider_and_hides_key(tmp_path, monkeypatch):
    monkeypatch.setenv('OPENAI_API_KEY', 'wrong-ambient-secret')
    monkeypatch.setenv('LITELLM_MODEL', 'openai/wrong')
    monkeypatch.setenv('GATEWAY_SECRET', 'synthetic-private-key')
    path = write_config(tmp_path, {'base_url': 'https://gateway.test/custom/v1/',
                                  'model': 'org/model', 'api_key_env': 'GATEWAY_SECRET'})
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(path))
    model = configured_model()
    assert model.model == 'openai/org/model'
    assert model.completion_args['api_base'] == 'https://gateway.test/custom/v1'
    assert model.completion_args['api_key'] == 'synthetic-private-key'
    assert 'synthetic-private-key' not in repr(model)
    settings = load_gateway(path)
    assert 'synthetic-private-key' not in repr(settings)
    assert settings.tls.check_hostname and settings.tls.verify_mode == ssl.CERT_REQUIRED


@pytest.mark.parametrize('change', [
    {'base_url': 'https://secret:password@gateway.test/v1'},
    {'base_url': 'https://@gateway.test/v1'},
    {'base_url': 'https://gateway.test/v1?key=secret'},
    {'base_url': 'file:///tmp/gateway'}, {'base_url': 'https://gateway.test:bad'},
    {'model': ''}, {'api_key_env': 'MISSING_GATEWAY_TEST_KEY'},
    {'tls': {'verify': False}}, {'tls': {'cert_file': 'client.pem'}},
    {'tls': {'ca_file': 'missing.pem'}}, {'api_key': 'inline-secret'},
])
def test_bad_configuration_fails_without_fallback(tmp_path, monkeypatch, change):
    monkeypatch.delenv('MISSING_GATEWAY_TEST_KEY', raising=False)
    path = write_config(tmp_path, {'base_url': 'https://gateway.test/v1', 'model': 'model', **change})
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(path))
    with pytest.raises(ValueError) as error:
        configured_model()
    assert 'inline-secret' not in str(error.value)
    assert 'password' not in str(error.value)


def test_disabled_missing_and_invalid_yaml_fail_closed(tmp_path, monkeypatch):
    path = write_config(tmp_path, {}, enabled=False)
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(path))
    with pytest.raises(ValueError, match='disabled'):
        configured_model()
    path.write_text('server: {}')
    with pytest.raises(ValueError, match='ai section'):
        configured_model()
    path.write_text('ai: [invalid: yaml')
    with pytest.raises(ValueError, match='configuration YAML'):
        configured_model()
    monkeypatch.delenv('THREADIFY_CONFIG_PATH')
    monkeypatch.setenv('LITELLM_MODEL', 'ollama_chat/local-model')
    assert configured_model().model == 'ollama_chat/local-model'


def test_agent_connection_configuration_is_separate_from_gateway(tmp_path):
    path = write_config(tmp_path, {'base_url': 'http://localhost:11434/v1', 'model': 'qwen'},
                        agent={'url': 'https://agent.example.test'})
    assert load_gateway(path).model == 'qwen'


def test_engine_style_environment_values(tmp_path, monkeypatch):
    monkeypatch.setenv('LOCAL_AI_URL', 'http://localhost:11434/v1')
    path = write_config(tmp_path, {'base_url': '$LOCAL_AI_URL', 'model': '$UNSET_TEST_MODEL:qwen'})
    assert load_gateway(path).model == 'qwen'
    assert load_gateway(path).base_url == 'http://localhost:11434/v1'


def certificate(tmp_path, name, issuer=None, usage=None):
    key = rsa.generate_private_key(public_exponent=65537, key_size=2048)
    subject = x509.Name([x509.NameAttribute(NameOID.COMMON_NAME, name)])
    builder = (x509.CertificateBuilder().subject_name(subject)
               .issuer_name(issuer[0].subject if issuer else subject)
               .public_key(key.public_key()).serial_number(x509.random_serial_number())
               .not_valid_before(datetime.now(timezone.utc) - timedelta(minutes=1))
               .not_valid_after(datetime.now(timezone.utc) + timedelta(days=1))
               .add_extension(x509.BasicConstraints(ca=issuer is None, path_length=None), critical=True))
    if usage:
        builder = builder.add_extension(x509.ExtendedKeyUsage([usage]), critical=False)
    if usage == ExtendedKeyUsageOID.SERVER_AUTH:
        builder = builder.add_extension(x509.SubjectAlternativeName([
            x509.DNSName('localhost'), x509.IPAddress(ipaddress.ip_address('127.0.0.1'))]), critical=False)
    cert = builder.sign(issuer[1] if issuer else key, hashes.SHA256())
    (tmp_path / f'{name}.pem').write_bytes(cert.public_bytes(serialization.Encoding.PEM))
    (tmp_path / f'{name}-key.pem').write_bytes(key.private_bytes(
        serialization.Encoding.PEM, serialization.PrivateFormat.PKCS8, serialization.NoEncryption()))
    return cert, key


@pytest.fixture
def gateway_server(tmp_path):
    ca = certificate(tmp_path, 'ca')
    certificate(tmp_path, 'server', ca, ExtendedKeyUsageOID.SERVER_AUTH)
    certificate(tmp_path, 'client', ca, ExtendedKeyUsageOID.CLIENT_AUTH)
    seen = []

    class Handler(BaseHTTPRequestHandler):
        def log_message(self, *args):
            pass

        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            seen.append((self.path, dict(self.headers), body, self.connection.getpeercert()))
            self.send_response(200)
            self.send_header('Content-Type', 'text/event-stream')
            self.end_headers()
            chunks = [
                {'index': 0, 'delta': {'role': 'assistant', 'tool_calls': [{'index': 0, 'id': 'tool-1',
                  'type': 'function', 'function': {'name': 'get_page_context', 'arguments': '{}'}}]}, 'finish_reason': None},
                {'index': 0, 'delta': {}, 'finish_reason': 'tool_calls'},
            ]
            for chunk in chunks:
                payload = {'id': 'test-completion', 'object': 'chat.completion.chunk',
                           'created': 1, 'model': body['model'], 'choices': [chunk]}
                self.wfile.write(('data: ' + json.dumps(payload) + '\n\n').encode())
            self.wfile.write(b'data: [DONE]\n\n')
            self.wfile.flush()

    server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
    tls = ssl.SSLContext(ssl.PROTOCOL_TLS_SERVER)
    tls.load_cert_chain(tmp_path / 'server.pem', tmp_path / 'server-key.pem')
    tls.load_verify_locations(tmp_path / 'ca.pem')
    tls.verify_mode = ssl.CERT_REQUIRED
    server.socket = tls.wrap_socket(server.socket, server_side=True)
    thread = threading.Thread(target=server.serve_forever, daemon=True)
    thread.start()
    yield f'https://127.0.0.1:{server.server_port}/custom/v1', seen
    server.shutdown()
    server.server_close()
    thread.join()


async def stream_model(model):
    adapter = model.build()
    try:
        result = await adapter.llm_client.acompletion(model=model.model,
            messages=[{'role': 'user', 'content': 'Read page context'}],
            tools=[{'type': 'function', 'function': {'name': 'get_page_context',
                'parameters': {'type': 'object', 'properties': {}}}}],
            stream=True, **dict(model.completion_args))
        return [chunk async for chunk in result]
    finally:
        await adapter.llm_client.aclose()


@pytest.mark.parametrize('authenticated', [False, True])
def test_real_mtls_gateway_streams_tool_calls(tmp_path, monkeypatch, gateway_server, authenticated):
    url, seen = gateway_server
    settings = {'base_url': url, 'model': 'org/local-model', 'tls': {
        'ca_file': 'ca.pem', 'cert_file': 'client.pem', 'key_file': 'client-key.pem'}}
    monkeypatch.setenv('OPENAI_API_KEY', 'must-not-leak')
    if authenticated:
        monkeypatch.setenv('TEST_AI_KEY', 'synthetic-gateway-key')
        settings['api_key_env'] = 'TEST_AI_KEY'
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(write_config(tmp_path, settings)))
    chunks = asyncio.run(stream_model(configured_model()))
    assert chunks[0].choices[0].delta.tool_calls[0].function.name == 'get_page_context'
    assert len(seen) == 1
    path, headers, body, cert = seen[0]
    assert path == '/custom/v1/chat/completions'
    assert body['model'] == 'org/local-model'
    assert body['tools'][0]['function']['name'] == 'get_page_context'
    assert cert['subject'][0][0][1] == 'client'
    assert headers.get('Authorization') == ('Bearer synthetic-gateway-key' if authenticated else None)
    assert 'must-not-leak' not in json.dumps(seen)


@pytest.mark.parametrize('tls', [{'ca_file': 'ca.pem'}, {'cert_file': 'client.pem', 'key_file': 'client-key.pem'}])
def test_tls_rejects_missing_client_certificate_or_untrusted_ca(tmp_path, monkeypatch, gateway_server, tls):
    url, seen = gateway_server
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(write_config(tmp_path, {
        'base_url': url, 'model': 'model', 'tls': tls})))
    # LiteLLM versions classify SDK TLS failures as either connection or 500 errors.
    with pytest.raises((litellm.APIConnectionError, litellm.InternalServerError), match="Connection error"):
        asyncio.run(stream_model(configured_model()))
    assert seen == []


def test_hosted_license_is_opt_in_and_reuses_engine_config(tmp_path, monkeypatch):
    monkeypatch.setenv('THREADIFY_LICENSE_KEY', 'environment-license')
    monkeypatch.setenv('THREADIFY_INSTALLATION_ID', 'environment-installation')
    path = write_config(tmp_path, {'base_url': 'https://ai.example.test/v1',
                                  'model': 'threadify-agent'})
    assert load_gateway(path).api_key == ''  # A custom gateway never receives the license.
    document = yaml.safe_load(path.read_text())
    document['ai']['gateway']['auth'] = 'threadify_license'
    document['registry'] = {'license_key': 'configured-license', 'installation_id': 'configured-installation'}
    path.write_text(yaml.safe_dump(document))
    settings = load_gateway(path)
    assert settings.api_key == 'configured-license'
    assert settings.installation_id == 'configured-installation'
    assert 'configured-license' not in repr(settings)
    del document['registry']
    path.write_text(yaml.safe_dump(document))
    assert load_gateway(path).api_key == 'environment-license'


@pytest.mark.parametrize('gateway', [
    {'auth': 'wrong'}, {'auth': []},
    {'auth': 'threadify_license', 'base_url': 'http://remote.example/v1'},
    {'auth': 'threadify_license', 'api_key_env': 'TEST_GATEWAY_KEY'},
    {'auth': 'threadify_license'},
])
def test_invalid_hosted_auth_fails_closed(tmp_path, monkeypatch, gateway):
    monkeypatch.delenv('THREADIFY_LICENSE_KEY', raising=False)
    monkeypatch.setenv('TEST_GATEWAY_KEY', 'conflicting-credential')
    path = write_config(tmp_path, {'base_url': 'https://ai.example.test/v1',
                                  'model': 'threadify-agent', **gateway})
    with pytest.raises(ValueError):
        load_gateway(path)


def test_hosted_transport_sends_license_and_installation(tmp_path, monkeypatch, gateway_server):
    url, seen = gateway_server
    monkeypatch.setenv('THREADIFY_LICENSE_KEY', 'synthetic-license')
    monkeypatch.setenv('THREADIFY_INSTALLATION_ID', 'synthetic-installation')
    path = write_config(tmp_path, {'base_url': url, 'model': 'threadify-agent',
        'auth': 'threadify_license', 'tls': {'ca_file': 'ca.pem',
        'cert_file': 'client.pem', 'key_file': 'client-key.pem'}})
    monkeypatch.setenv('THREADIFY_CONFIG_PATH', str(path))
    asyncio.run(stream_model(configured_model()))
    assert seen[0][1]['Authorization'] == 'Bearer synthetic-license'
    assert seen[0][1]['X-Threadify-Installation-ID'] == 'synthetic-installation'
