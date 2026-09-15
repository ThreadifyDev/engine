"""Opt-in non-billing CRUD against a running local Engine and Web API.

THREADIFY_LIVE_BEARER_FILE=/path/to/token python3 -m unittest discover -s live -v
Uses unique disposable records, removes its records, and never sends email.
"""
import json
import os
from pathlib import Path
import unittest
import urllib.error
import urllib.request
import urllib.parse
import uuid


@unittest.skipUnless(os.environ.get('THREADIFY_LIVE_BEARER_FILE'), 'requires explicit local bearer file')
class ManagementLifecycle(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.token = Path(os.environ['THREADIFY_LIVE_BEARER_FILE']).read_text().strip()
        cls.api = os.environ.get('THREADIFY_LIVE_API_URL', 'http://127.0.0.1:3003')
        cls.engine = os.environ.get('THREADIFY_LIVE_ENGINE_URL', 'http://127.0.0.1:8083')
        for url in (cls.api, cls.engine):
            if urllib.parse.urlparse(url).hostname not in ('127.0.0.1', 'localhost', '::1'):
                raise ValueError('This disposable-fixture test is restricted to local services')

    def request(self, method, path, body=None, status=200, engine=False, authenticated=True):
        headers = {'Authorization': 'Bearer ' + self.token} if authenticated else {}
        data = None
        if body is not None:
            data = body.encode() if isinstance(body, str) else json.dumps(body).encode()
            headers['Content-Type'] = 'text/plain' if isinstance(body, str) else 'application/json'
        req = urllib.request.Request((self.engine if engine else self.api) + path, data=data, headers=headers, method=method)
        try:
            response = urllib.request.urlopen(req, timeout=20)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            raw = response.read()
            # Avoid displaying response bodies: successful key creation returns a secret.
            self.assertEqual(response.status, status, f'{method} {path}: unexpected HTTP status')
            return json.loads(raw) if raw else {}

    def test_profile_type_lifecycle(self):
        name = 'Lifecycle ' + uuid.uuid4().hex[:12]
        slug = name.lower().replace(' ', '_')
        path = '/api/entity-profile-types/' + slug
        declaration = {'name': name, 'type': [slug], 'metrics': []}
        planned = self.request('PUT', path + '?dry_run=true', declaration)
        self.assertTrue(planned['dry_run'])
        self.assertNotIn(slug, [x['slug'] for x in self.request('GET', '/api/entity-profile-types')['data'] or []])
        created = self.request('PUT', path, declaration)['data']
        try:
            self.assertEqual(created.get('description', ''), '')
            listed = self.request('GET', '/api/entity-profile-types')['data']
            self.assertIn(created['id'], [x['id'] for x in listed])
            declaration['description'] = 'Updated by the non-billing lifecycle test'
            updated = self.request('PUT', path, declaration)['data']
            self.assertEqual(updated['id'], created['id'])
            self.assertEqual(updated['description'], declaration['description'])
            renamed = self.request('POST', path + '/rename', {'name': name + ' Renamed'})['data']
            path = '/api/entity-profile-types/' + renamed['slug']
            self.assertEqual(renamed['id'], created['id'])
        finally:
            self.request('DELETE', path)
        self.assertNotIn(created['id'], [x['id'] for x in self.request('GET', '/api/entity-profile-types')['data'] or []])

    def test_service_account_lifecycle(self):
        name = 'lifecycle-' + uuid.uuid4().hex[:12]
        created = self.request('POST', '/api/service-accounts', {'name': name, 'role': 'reader'}, status=201)['service_account']
        path = '/api/service-accounts/' + created['id']
        try:
            self.assertEqual(self.request('GET', path)['service_account']['name'], name)
            self.request('PUT', path, {'name': name + '-updated', 'description': 'Lifecycle check', 'is_active': False})
            fetched = self.request('GET', path)['service_account']
            self.assertEqual(fetched['name'], name + '-updated')
            self.assertFalse(fetched['is_active'])
        finally:
            self.request('DELETE', path)
        self.request('GET', path, status=404)

    def test_api_key_lifecycle(self):
        result = self.request('POST', '/api/api-keys', {'name': 'lifecycle-' + uuid.uuid4().hex[:12], 'expires_in': 1}, status=201)
        key_id = result['api_key']['id']
        try:
            self.assertTrue(result['key'])
            self.assertTrue(result['api_key']['expires_at'])
            self.assertIn(key_id, [x['id'] for x in self.request('GET', '/api/api-keys')['api_keys'] or []])
        finally:
            self.request('DELETE', '/api/api-keys/' + key_id)
        self.assertNotIn(key_id, [x['id'] for x in self.request('GET', '/api/api-keys')['api_keys'] or []])

    def test_contract_lifecycle(self):
        name = 'lifecycle_' + uuid.uuid4().hex[:12]
        contract = f'''contract_name: {name}
version: 1
description: Lifecycle check
parties: [worker]
steps:
  - id: received
    owner: worker
    type: managed
  - id: completed
    owner: worker
    type: managed
transitions:
  - from: received
    to: [completed]
entry_points: [received]
terminal_steps: [completed]
validation:
  max_duration: 1h
versioning:
  threads_lock_to_version: true
'''
        self.request('POST', '/v1/contracts/preview', contract, engine=True)
        result = self.request('POST', '/v1/contracts', contract, engine=True)
        path = '/v1/contracts/' + result['contract']['id']
        try:
            self.assertEqual(self.request('GET', path, engine=True)['contract']['name'], name)
            revised = contract.replace('version: 1', 'version: 2').replace('description: Lifecycle check', 'description: Updated lifecycle check')
            unchanged = self.request('PUT', path, revised, engine=True, status=400)
            self.assertEqual(unchanged['message'], 'Contract content has not changed')
            revised = revised.replace('max_duration: 1h', 'max_duration: 2h')
            self.request('PUT', path, revised, engine=True)
            fetched = self.request('GET', path, engine=True)
            self.assertEqual(fetched['contract']['latestVersion'], 2)
            self.assertEqual(fetched['contract']['description'], 'Updated lifecycle check')
        finally:
            self.request('DELETE', path, engine=True)
        self.request('GET', path, engine=True, status=404)

    def test_account_and_team_reads(self):
        profile = self.request('GET', '/api/user/profile?minimal=true')
        self.assertTrue(profile)
        for path in ('/api/team/members', '/api/team/invitations', '/api/service-accounts', '/api/api-keys'):
            with self.subTest(path=path):
                self.request('GET', path)

    def test_authentication_required(self):
        for path in ('/api/entity-profile-types', '/api/team/members', '/api/service-accounts', '/api/api-keys'):
            with self.subTest(path=path):
                self.request('GET', path, authenticated=False, status=401)
        self.request('GET', '/v1/contracts', engine=True, authenticated=False, status=401)


if __name__ == '__main__':
    unittest.main()
