#!/usr/bin/env -S python3 -I
"""Run an explicit corpus command with Batz-scoped 1Password credentials."""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
KEYCHAIN_SERVICE = 'batz-corpus-op-service-account'
REQUIRED = ('BATZ_CORPUS_R2_BUCKET', 'BATZ_CORPUS_R2_ENDPOINT',
            'BATZ_CORPUS_R2_ACCESS_KEY_ID', 'BATZ_CORPUS_R2_SECRET_ACCESS_KEY')


class CredentialsError(Exception):
    pass


def validate_references(path):
    """Bucket/endpoint metadata may be literal; credentials stay in the Batz vault."""
    values = {}
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith('#'):
            continue
        key, separator, value = line.partition('=')
        key, value = key.strip(), value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in '\"\'':
            value = value[1:-1]
        if not separator or key not in REQUIRED or key in values:
            raise CredentialsError('Invalid corpus reference file.')
        reference = re.fullmatch(r'op://Batz/[^\s$"\x27`]+/[^$"\x27`]+', value)
        bucket = key == 'BATZ_CORPUS_R2_BUCKET' and re.fullmatch(r'[a-z0-9][a-z0-9-]{1,61}[a-z0-9]', value)
        endpoint = key == 'BATZ_CORPUS_R2_ENDPOINT' and re.fullmatch(
            r'https://[a-z0-9.-]+\.r2\.cloudflarestorage\.com/?', value)
        if not reference and not bucket and not endpoint:
            raise CredentialsError('Corpus credentials must use Batz-vault secret references.')
        values[key] = value
    if set(values) != set(REQUIRED):
        raise CredentialsError('Corpus reference file is incomplete.')


def captured(argv, env):
    """Keep credential-bearing output and third-party diagnostics private."""
    try:
        result = subprocess.run(argv, env=env, capture_output=True, text=True, timeout=90)
    except (OSError, subprocess.TimeoutExpired):
        raise CredentialsError('Credential provider unavailable.') from None
    if result.returncode:
        raise CredentialsError('Credential provider failed; no credentials were displayed.')
    return result.stdout


def main():
    args = sys.argv[1:]
    if args == ['check']:
        command = [sys.executable, '-I', '-c',
                   'import os,sys; '
                   f'missing=[name for name in {REQUIRED!r} if not os.environ.get(name)]; '
                   'print("Batz corpus credentials incomplete." if missing else '
                   '"Batz corpus credentials resolved."); sys.exit(bool(missing))']
    elif len(args) > 1 and args[0] == '--':
        command = args[1:]
    else:
        print('Usage: python3 -I ci/corpus-credentials.py check | -- COMMAND [ARG ...]', file=sys.stderr)
        return 2
    try:
        refs = Path(os.environ.get('BATZ_CORPUS_OP_ENV', ROOT / 'corpus.op.env'))
        if not refs.is_file():
            raise CredentialsError('Missing corpus.op.env; see docs/operations/corpus-access.md.')
        validate_references(refs)
        if not shutil.which('op'):
            raise CredentialsError('The 1Password CLI (op) is required.')
        env = dict(os.environ)
        env.pop('OP_CONNECT_HOST', None)
        env.pop('OP_CONNECT_TOKEN', None)
        token = env.get('OP_SERVICE_ACCOUNT_TOKEN')
        if not token:
            token = captured(['security', 'find-generic-password', '-s', KEYCHAIN_SERVICE, '-w'], env).strip()
        if not token:
            raise CredentialsError('Missing Batz service-account token.')
        env['OP_SERVICE_ACCOUNT_TOKEN'] = token
        try:
            vaults = json.loads(captured(['op', 'vault', 'list', '--format=json'], env))
        except ValueError:
            raise CredentialsError('Cannot verify Batz vault access.') from None
        if (not isinstance(vaults, list) or len(vaults) != 1 or not isinstance(vaults[0], dict)
                or vaults[0].get('name', '').casefold() != 'batz'):
            raise CredentialsError('Service account must have access to the Batz vault only.')
        for name in REQUIRED:
            env.pop(name, None)
        # op retains its token; the actual consumer receives only resolved credentials.
        argv = ['op', 'run', '--env-file', str(refs), '--', '/usr/bin/env',
                '-u', 'OP_SERVICE_ACCOUNT_TOKEN', '-u', 'OP_CONNECT_HOST', '-u', 'OP_CONNECT_TOKEN',
                *command]
        os.execvpe('op', argv, env)
    except (CredentialsError, OSError) as error:
        message = str(error) if isinstance(error, CredentialsError) else 'Unable to launch corpus command.'
        print(message, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())
