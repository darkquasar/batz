#!/usr/bin/env python3
"""Prepare pinned dependencies in a fresh macOS runner; tests stay offline."""
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
        subprocess.run(['curl', '--fail', '--location', '--retry', '3',
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
    local = Path(os.environ['BATZ_TOOLCHAIN_ROOT']).resolve()
    cache = Path(os.environ['BATZ_DOWNLOAD_CACHE']).resolve()
    # Never overwrite a developer's installed runtime or reuse an installed venv.
    local.mkdir(parents=True, exist_ok=False)
    cache.mkdir(parents=True, exist_ok=True)
    lock = json.loads((ROOT / 'runtime/dependencies.lock.json').read_text())
    for name, expected in lock['lock_files'].items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != expected:
            raise ValueError('lock file checksum mismatch: ' + name)
    go = next(a for a in lock['go']['artifacts'] if '.darwin-arm64.' in a['filename'])
    python = lock['python']['artifacts']['cpython-' + lock['python']['version'] + '-darwin-aarch64-none']
    for name, artifact in [('go', go), ('python', python)]:
        archive = cache / (artifact['sha256'] + '.tar.gz')
        verified_download(artifact['url'], archive, artifact['sha256'])
        subprocess.run(['tar', '-xzf', str(archive), '-C', str(local)], check=True)
    py = local / 'python/bin/python3.12'
    expected_python = lock['installed_binaries']['darwin/arm64']['python']['sha256']
    if hashlib.sha256(py.read_bytes()).hexdigest() != expected_python:
        raise ValueError('installed Python checksum mismatch')
    subprocess.run([str(py), '-I', '-m', 'venv', str(local / 'venv')], check=True)
    env = dict(os.environ, PIP_CACHE_DIR=str(cache / 'pip'),
               PIP_DISABLE_PIP_VERSION_CHECK='1', PYTHONDONTWRITEBYTECODE='1',
               GOTOOLCHAIN='local', GOPROXY='https://proxy.golang.org', GOSUMDB='sum.golang.org')
    subprocess.run([str(local / 'venv/bin/python'), '-I', '-m', 'pip', 'install',
                    '--require-hashes', '--only-binary=:all:',
                    '-r', str(ROOT / 'runtime/requirements.lock'),
                    '-r', str(ROOT / 'runtime/check-requirements.lock')], env=env, check=True)
    subprocess.run([str(local / 'go/bin/go'), 'mod', 'download'], cwd=ROOT, env=env, check=True)
    subprocess.run([str(local / 'go/bin/go'), 'mod', 'verify'], cwd=ROOT, env=env, check=True)
    print(json.dumps({'kind': 'dependency-setup', 'seconds': round(time.monotonic() - started, 3)}), flush=True)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
