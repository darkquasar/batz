#!/usr/bin/env python3
"""Locked mise postinstall: prepare only this installation's private uv runtime."""
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent


def sha256(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def clean_environment(root):
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('PYTHON', 'UV_', 'PIP_', 'CONDA_')) and key != 'VIRTUAL_ENV'
           and (not key.startswith('GO') or key in ('GOCACHE', 'GOMODCACHE', 'GOPATH'))}
    env.update(UV_PROJECT_ENVIRONMENT=str(root / 'runtime/.local/venv'),
               UV_CACHE_DIR=str(root / 'runtime/.local/uv-cache'),
               PYTHONDONTWRITEBYTECODE='1', GOTOOLCHAIN='local', GOENV='off', GOWORK='off',
               GOFLAGS='', GOEXPERIMENT='', GOCACHEPROG='', CGO_ENABLED='0', GOMAXPROCS='2')
    return env


def target_name():
    target = {('Darwin', 'arm64'): 'darwin/arm64', ('Linux', 'x86_64'): 'linux/amd64'}.get(
        (platform.system(), platform.machine()))
    if not target:
        raise ValueError('unsupported native setup target')
    return target


def owned_runtime_paths(root):
    for relative in ('runtime', 'runtime/.local', 'runtime/.local/python', 'runtime/.local/venv'):
        path = root / relative
        if path.is_symlink():
            raise ValueError('foreign runtime path: ' + str(path))


def verify_go_binary(go_root, lock, target):
    if sha256(go_root / 'bin/go') != lock['go']['binary_sha256'][target]:
        raise ValueError('mise Go binary checksum mismatch')


def verify_installation(root=ROOT):
    root = root.resolve()
    owned_runtime_paths(root)
    lock_path = root / 'runtime/dependencies.lock.json'
    lock = json.loads(lock_path.read_text())
    for name, expected in lock['lock_files'].items():
        if sha256(root / name) != expected:
            raise ValueError('lock file checksum mismatch: ' + name)
    receipt = json.loads((root / 'runtime/.local/installation.json').read_text())
    target = target_name()
    go_root = root / 'runtime/.local/go'
    if go_root.exists() or go_root.is_symlink():
        verify_go_binary(go_root, lock, target)
    expected = lock['python']['binary_sha256'][target]
    artifact = receipt['binaries']['python']
    if (receipt['schema'] != 'batz-installation/v0alpha1' or receipt['target'] != target
            or receipt['runtime_lock_sha256'] != sha256(lock_path)
            or artifact['path'] != 'runtime/.local/venv/bin/python' or artifact['sha256'] != expected
            or sha256(root / artifact['path']) != expected):
        raise ValueError('runtime installation integrity mismatch; run mise install')
    venv = root / 'runtime/.local/venv'
    config = dict(line.split(' = ', 1) for line in (venv / 'pyvenv.cfg').read_text().splitlines()
                  if ' = ' in line)
    python = root / 'runtime/.local/python/bin/python3.12'
    if (config.get('uv') != lock['uv']['version']
            or config.get('include-system-site-packages', '').lower() != 'false'
            or Path(config.get('home', '')).resolve() != python.parent.resolve()
            or (venv / 'bin/python').resolve() != python.resolve()):
        raise ValueError('environment must be a private uv environment using owned Python')
    if (root / '.git').exists():
        collector = receipt['binaries'].get('collector', {})
        if (receipt.get('collector_source_sha256') != collector_source_sha256(root)
                or collector.get('path') != 'runtime/.local/batz-collector'
                or collector.get('sha256') != sha256(root / 'runtime/.local/batz-collector')):
            raise ValueError('collector source or binary changed; run mise install')
    return lock


def collector_source_sha256(root):
    paths = [root / 'go.mod', root / 'go.sum']
    for directory in ('collector', 'cmd/batz-collector', 'internal/buildinfo'):
        paths.extend((root / directory).rglob('*.go'))
    digest = hashlib.sha256()
    for path in sorted(paths, key=lambda path: path.relative_to(root).as_posix()):
        if path.is_symlink() or not path.is_file():
            raise ValueError('invalid collector source: ' + str(path))
        digest.update(path.relative_to(root).as_posix().encode() + b'\0' + path.read_bytes() + b'\0')
    return digest.hexdigest()


def main(root=ROOT):
    root = root.resolve()
    local = root / 'runtime/.local'
    owned_runtime_paths(root)
    lock_path = root / 'runtime/dependencies.lock.json'
    lock = json.loads(lock_path.read_text())
    for name, expected in lock['lock_files'].items():
        if sha256(root / name) != expected:
            raise ValueError('lock file checksum mismatch: ' + name)
    target = target_name()
    expected = lock['python']['binary_sha256'][target]
    # The hook is launched with mise's Python, never an ambient/system Python.
    source = Path(sys.executable).resolve()
    if sha256(source) != expected:
        raise ValueError('mise Python binary checksum mismatch')
    env = clean_environment(root)
    mise = os.environ.get('MISE_BIN', 'mise')
    version = subprocess.check_output([mise, 'version'], cwd=root, env=env, text=True).split()[0]
    if version != lock['mise']['version']:
        raise ValueError('mise version does not match runtime lock')
    install_path = os.environ.get('MISE_TOOL_INSTALL_PATH')
    uv_root = Path(install_path) if install_path else Path(subprocess.check_output(
        [mise, 'where', 'http:batz-uv@' + lock['uv']['version']], cwd=root, env=env, text=True).strip())
    uv = uv_root / 'uv'
    if sha256(uv) != lock['uv']['binary_sha256'][target]:
        raise ValueError('mise uv binary checksum mismatch')
    local.mkdir(parents=True, exist_ok=True)
    go_root = Path(subprocess.check_output([mise, 'where', 'http:batz-go@' + lock['go']['version']],
                                          cwd=root, env=env, text=True).strip())
    verify_go_binary(go_root, lock, target)
    go_path = local / 'go'
    if not go_path.exists():
        go_path.symlink_to(go_root, target_is_directory=True)
    elif go_path.resolve() != go_root.resolve():
        raise ValueError('foreign Go toolchain path')
    python_root = local / 'python'
    if not python_root.exists():
        shutil.copytree(source.parent.parent, python_root, symlinks=True)
    python = python_root / 'bin/python3.12'
    if sha256(python) != expected:
        raise ValueError('private Python binary checksum mismatch')
    venv = local / 'venv'
    if venv.exists():
        config = (venv / 'pyvenv.cfg').read_text()
        if 'include-system-site-packages = true' in config.lower():
            raise ValueError('foreign system site packages are forbidden')
        # Recreate the owned environment, including legacy stdlib environments,
        # so undeclared files/site hooks cannot survive a locked reinstall.
        shutil.rmtree(venv)
    receipt = local / 'installation.json'
    receipt.unlink(missing_ok=True)
    subprocess.run([str(uv), '--no-config', 'sync', '--locked', '--no-python-downloads',
                    '--no-build', '--reinstall', '--refresh', '--no-install-project', '--all-groups', '--project', str(root),
                    '--python', str(python)], cwd=root, env=env, check=True)
    installed = venv / 'bin/python'
    if sha256(installed) != expected or installed.resolve() != python.resolve():
        raise ValueError('uv environment Python identity mismatch')
    config = (venv / 'pyvenv.cfg').read_text().lower()
    if 'include-system-site-packages = false' not in config or 'uv = ' not in config:
        raise ValueError('environment must be private and uv-created')
    observation = {'schema': 'batz-installation/v0alpha1', 'target': target,
                   'runtime_lock_sha256': sha256(lock_path),
                   'binaries': {'python': {'path': 'runtime/.local/venv/bin/python', 'sha256': expected}}}
    if (root / '.git').exists():
        go = go_root / 'bin/go'
        env.update(GOTOOLCHAIN='local', GOENV='off', GOWORK='off', GOFLAGS='',
                   GOPROXY='https://proxy.golang.org', GOSUMDB='sum.golang.org', GOMAXPROCS='2')
        for variable, name in (('GOMODCACHE', 'gomod'), ('GOCACHE', 'gocache'), ('GOPATH', 'gopath')):
            env.setdefault(variable, str(local / name))
        for operation in ('download', 'verify'):
            subprocess.run([str(go), 'mod', operation], cwd=root, env=env, check=True)
        collector = local / 'batz-collector'
        temporary = local / 'batz-collector.partial'
        source_digest = collector_source_sha256(root)
        subprocess.run([str(go), 'build', '-trimpath', '-p', '1', '-o', str(temporary),
                        './cmd/batz-collector'], cwd=root, env=env, check=True)
        if collector_source_sha256(root) != source_digest:
            raise ValueError('collector source changed during setup')
        temporary.replace(collector)
        observation['collector_source_sha256'] = source_digest
        observation['binaries']['collector'] = {'path': 'runtime/.local/batz-collector',
                                                'sha256': sha256(collector)}
    partial = receipt.with_suffix('.partial')
    partial.write_text(json.dumps(observation, indent=2) + '\n')
    partial.replace(receipt)
    print(json.dumps({'kind': 'runtime-setup', 'target': target, 'environment': str(venv)}), flush=True)


if __name__ == '__main__':
    try:
        if sys.argv[1:] == ['--check']:
            verify_installation()
        elif sys.argv[1:]:
            raise ValueError('usage: setup.py [--check]')
        else:
            main()
    except (ValueError, OSError, KeyError, subprocess.CalledProcessError) as error:
        sys.exit(str(error))
