#!/usr/bin/env python3
"""Hosted macOS bootstrap; normal trusted mise setup precedes offline checks."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parent.parent


def verified_download(url, destination, expected):
    if not destination.exists():
        partial = destination.with_suffix('.partial')
        subprocess.run(['curl', '--fail', '--location', '--retry', '3', '--retry-all-errors',
                        '--output', str(partial), url], check=True)
        if hashlib.sha256(partial.read_bytes()).hexdigest() != expected:
            raise ValueError('download checksum mismatch: ' + destination.name)
        partial.replace(destination)
    if hashlib.sha256(destination.read_bytes()).hexdigest() != expected:
        raise ValueError('cached download checksum mismatch: ' + destination.name)


def main():
    started = time.monotonic()
    if (platform.system(), platform.machine()) != ('Darwin', 'arm64'):
        raise ValueError('preparation requires native macOS ARM64')
    local = ROOT / 'runtime/.local'
    cache = Path(os.environ.get('BATZ_DOWNLOAD_CACHE', local / 'downloads')).resolve()
    local.mkdir(parents=True, exist_ok=True)
    cache.mkdir(parents=True, exist_ok=True)
    lock = json.loads((ROOT / 'runtime/dependencies.lock.json').read_text())
    artifact = lock['mise']['artifacts']['darwin/arm64']
    mise = cache / ('mise-' + lock['mise']['version'])
    verified_download(artifact['url'], mise, artifact['sha256'])
    mise.chmod(0o755)
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('PYTHON', 'UV_', 'PIP_', 'CONDA_')) and key != 'VIRTUAL_ENV'}
    env.update(MISE_DATA_DIR=str(local/'mise/data'), MISE_CACHE_DIR=str(cache/'mise'),
               MISE_CONFIG_DIR=str(local/'mise/config'), MISE_STATE_DIR=str(local/'mise/state'),
               MISE_BIN=str(mise), GOTOOLCHAIN='local', GOPROXY='https://proxy.golang.org',
               GOSUMDB='sum.golang.org')
    subprocess.run([str(mise), 'trust', str(ROOT / 'mise.toml')], cwd=ROOT, env=env, check=True)
    subprocess.run([str(mise), 'install'], cwd=ROOT, env=env, check=True)
    print(json.dumps({'kind': 'dependency-setup', 'seconds': round(time.monotonic() - started, 3)}), flush=True)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
