"""Credential launcher behavior with invented secrets and external CLI doubles."""
from pathlib import Path
import os
import subprocess
import sys
import tempfile
import unittest

LAUNCHER = Path(__file__).with_name('corpus-credentials.py')
REQUIRED = ('BATZ_CORPUS_R2_BUCKET', 'BATZ_CORPUS_R2_ENDPOINT',
            'BATZ_CORPUS_R2_ACCESS_KEY_ID', 'BATZ_CORPUS_R2_SECRET_ACCESS_KEY')


class CorpusCredentials(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name)
        self.refs = self.root / 'corpus.op.env'
        self.refs.write_text(''.join(f'{key}="op://Batz/invented-item/{key}"\n' for key in REQUIRED))
        self.env = {**os.environ, 'PATH': str(self.root) + os.pathsep + os.environ['PATH'],
                    'BATZ_CORPUS_OP_ENV': str(self.refs),
                    'OP_CONNECT_HOST': 'https://invented-connect.invalid',
                    'OP_CONNECT_TOKEN': 'invented-connect-secret'}
        self.env.pop('OP_SERVICE_ACCOUNT_TOKEN', None)
        self.tool('security', "print('invented-service-secret')")
        self.tool('op', '''
import json, os, subprocess, sys
assert os.environ['OP_SERVICE_ACCOUNT_TOKEN'] == 'invented-service-secret'
assert 'OP_CONNECT_HOST' not in os.environ and 'OP_CONNECT_TOKEN' not in os.environ
if sys.argv[1:3] == ['vault', 'list']:
    print(json.dumps(json.loads(os.environ.get('INVENTED_VAULTS', '[{"id":"invented-vault","name":"Batz"}]'))))
else:
    assert sys.argv[1] == 'run'
    command = sys.argv[sys.argv.index('--') + 1:]
    child = dict(os.environ)
    for name in ('BATZ_CORPUS_R2_BUCKET', 'BATZ_CORPUS_R2_ENDPOINT',
                 'BATZ_CORPUS_R2_ACCESS_KEY_ID', 'BATZ_CORPUS_R2_SECRET_ACCESS_KEY'):
        child[name] = 'invented-r2-secret'
    sys.exit(subprocess.run(command, env=child).returncode)
''')

    def tool(self, name, body):
        path = self.root / name
        path.write_text(f'#!{sys.executable}\n{body}\n')
        path.chmod(0o700)

    def launch(self, *args):
        return subprocess.run([sys.executable, '-I', str(LAUNCHER), *args],
                              env=self.env, capture_output=True, text=True)

    def test_scoped_keychain_credentials_reach_command_without_service_token(self):
        result = self.launch('--', sys.executable, '-c',
                             'import os; assert "OP_SERVICE_ACCOUNT_TOKEN" not in os.environ; '
                             'assert "OP_CONNECT_TOKEN" not in os.environ; '
                             'assert os.environ["BATZ_CORPUS_R2_BUCKET"] == "invented-r2-secret"; '
                             'print("consumer authenticated")')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), 'consumer authenticated')
        self.assertNotIn('invented-service-secret', result.stdout + result.stderr)

    def test_plaintext_or_foreign_vault_secret_references_are_refused(self):
        for reference in ('invented-plaintext-secret', 'op://AnotherVault/item/key'):
            with self.subTest(reference=reference):
                self.refs.write_text(''.join(
                    f'{key}="{reference if key.endswith("SECRET_ACCESS_KEY") else "op://Batz/item/field"}"\n'
                    for key in REQUIRED))
                result = self.launch('--', sys.executable, '-c', 'print("consumer started")')
                self.assertEqual(result.returncode, 2)
                self.assertNotIn('consumer started', result.stdout)
                self.assertNotIn(reference, result.stdout + result.stderr)

    def test_broader_or_wrong_service_account_never_starts_consumer(self):
        import json
        for vaults in ([{'name': 'Batz'}, {'name': 'AnotherVault'}], [{'name': 'AnotherVault'}]):
            with self.subTest(vaults=vaults):
                self.env['INVENTED_VAULTS'] = json.dumps(vaults)
                result = self.launch('--', sys.executable, '-c', 'print("consumer started")')
                self.assertEqual(result.returncode, 2)
                self.assertNotIn('consumer started', result.stdout)
                self.assertIn('Batz vault only', result.stderr)

    def test_provider_diagnostics_cannot_disclose_credentials(self):
        self.tool('op', 'import sys; print("invented-service-secret", file=sys.stderr); sys.exit(1)')
        result = self.launch('check')
        self.assertEqual(result.returncode, 2)
        self.assertNotIn('invented-service-secret', result.stdout + result.stderr)

    def test_absent_keychain_token_does_not_fall_back_to_desktop_access(self):
        self.tool('security', 'import sys; sys.exit(44)')
        result = self.launch('--', sys.executable, '-c', 'print("consumer started")')
        self.assertEqual(result.returncode, 2)
        self.assertNotIn('consumer started', result.stdout)

    def test_explicit_ci_token_is_scoped_and_does_not_require_keychain(self):
        self.env['OP_SERVICE_ACCOUNT_TOKEN'] = 'invented-service-secret'
        self.tool('security', 'raise RuntimeError("keychain must not be read")')
        result = self.launch('check')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), 'Batz corpus credentials resolved.')

    def test_consumer_exit_status_is_preserved(self):
        result = self.launch('--', sys.executable, '-c', 'raise SystemExit(7)')
        self.assertEqual(result.returncode, 7)

    def test_nonsecret_bucket_and_r2_endpoint_can_be_local_metadata(self):
        self.refs.write_text(
            'BATZ_CORPUS_R2_BUCKET="invented-batz-corpus"\n'
            'BATZ_CORPUS_R2_ENDPOINT="https://example.r2.cloudflarestorage.com"\n'
            'BATZ_CORPUS_R2_ACCESS_KEY_ID="op://Batz/item/key"\n'
            'BATZ_CORPUS_R2_SECRET_ACCESS_KEY="op://Batz/item/secret"\n')
        result = self.launch('check')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)


if __name__ == '__main__':
    unittest.main()
