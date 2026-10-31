#!/usr/bin/env python3
"""Shared local/CI commands around Go, unittest and native SQLMesh."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager
import threading
import fcntl
import hashlib
import json
import os
from pathlib import Path
import platform
import shutil
import subprocess
import sys
import tempfile
import time

# -I deliberately excludes the working directory from Python's import path.
sys.path.insert(0, str(Path(__file__).resolve().parent))
from pipelines import ROOT, reference, tag, working_catalog


_worker = threading.local()


def command(argv, kind='test'):
    started = time.monotonic()
    print('+ ' + ' '.join(map(str, argv)), flush=True)
    result = subprocess.run(list(map(str, argv)), cwd=ROOT, pass_fds=getattr(_worker, 'fds', ()))
    print(json.dumps({'kind': kind, 'command': list(map(str, argv)),
                      'exit': result.returncode, 'seconds': round(time.monotonic()-started, 3)}), flush=True)
    if result.returncode:
        raise subprocess.CalledProcessError(result.returncode, argv)


def environment():
    local = Path(os.environ.get('BATZ_TOOLCHAIN_ROOT', ROOT / 'runtime/.local'))
    if (local / 'go/bin/go').is_file():
        os.environ['PATH'] = str(local / 'go/bin') + os.pathsep + os.environ.get('PATH', '')
    os.environ.update(GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off', GOMAXPROCS='2',
                      PYTHONDONTWRITEBYTECODE='1')
    for variable, directory in [('GOCACHE', 'gocache'), ('GOPATH', 'gopath'), ('GOMODCACHE', 'gomod')]:
        if (local / directory).is_dir():
            os.environ.setdefault(variable, str(local / directory))
    if not shutil.which('go'):
        raise FileNotFoundError('missing pinned Go toolchain')
    lock = json.loads((ROOT / 'runtime/dependencies.lock.json').read_text())
    actual = subprocess.check_output(['go', 'version'], text=True)
    if 'go' + lock['go']['version'] + ' ' not in actual:
        raise ValueError('Go toolchain does not match runtime lock: ' + actual.strip())
    if platform.python_version() != lock['python']['version']:
        raise ValueError('Python does not match runtime lock')
    return lock


@contextmanager
def prepare_native(lock):
    if (platform.system(), platform.machine()) != ('Darwin', 'arm64'):
        raise ValueError('native lanes require the qualified macOS ARM64 host')
    local = Path(os.environ.get('BATZ_TOOLCHAIN_ROOT', ROOT / 'runtime/.local')).resolve()
    python = local / 'venv/bin/python'
    installed = lock['installed_binaries']['darwin/arm64']
    if hashlib.sha256(python.read_bytes()).hexdigest() != installed['python']['sha256']:
        raise ValueError('pinned Python binary checksum mismatch')
    # Each invocation owns its build and receipt. Installed assets and the
    # reviewed production lock are never rewritten by test execution.
    with tempfile.TemporaryDirectory(prefix='batz-test-runtime-') as scratch:
        private = Path(scratch)
        built = private / 'batz-collector'
        command(['go', 'build', '-p', '1', '-o', built, './cmd/batz-collector'], kind='setup')
        for name in ('venv', 'python'):
            (private / name).symlink_to(local / name, target_is_directory=True)
        receipt = json.loads(json.dumps(lock))
        receipt['installed_binaries']['darwin/arm64']['collector']['sha256'] = hashlib.sha256(built.read_bytes()).hexdigest()
        (private / 'dependencies.lock.json').write_text(json.dumps(receipt, indent=2) + '\n')
        os.environ['BATZ_TEST_RUNTIME'] = str(private)
        try:
            yield
        finally:
            os.environ.pop('BATZ_TEST_RUNTIME', None)


@contextmanager
def native_slots(count, locks_root=None):
    if count not in (0, 1, 2):
        raise ValueError('native worker budget must be 0, 1 or 2')
    if count == 0:
        yield
        return
    directory = locks_root or Path(tempfile.gettempdir()) / ('batz-native-workers-' + str(os.getuid()))
    directory.mkdir(mode=0o700, parents=True, exist_ok=True)
    leases = []
    while True:
        try:
            for index in range(2):
                lease = (directory / ('slot-' + str(index))).open('a')
                try:
                    fcntl.flock(lease, fcntl.LOCK_EX | fcntl.LOCK_NB)
                except BlockingIOError:
                    lease.close()
                    continue
                leases.append(lease)
                if len(leases) == count:
                    break
            if len(leases) == count:
                break
        finally:
            if len(leases) < count:
                for lease in leases:
                    lease.close()
                leases = []
        time.sleep(0.01)
    previous = getattr(_worker, 'fds', ())
    _worker.fds = tuple(lease.fileno() for lease in leases)
    try:
        yield
    finally:
        _worker.fds = previous
        for lease in leases:
            lease.close()


def run_jobs(jobs, locks_root=None):
    def execute(slots, work):
        with native_slots(slots, locks_root):
            return work()

    with ThreadPoolExecutor(max_workers=2) as workers:
        futures = [workers.submit(execute, slots, work) for slots, work in jobs]
        for future in futures:
            future.result()


def whitespace():
    # CI passes its target SHA and fetches full history. Locally include staged,
    # unstaged and new source files against HEAD, without a result cache.
    base = os.environ.get('BATZ_DIFF_BASE', 'HEAD')
    common = subprocess.check_output(['git', 'merge-base', base, 'HEAD'], cwd=ROOT, text=True).strip()
    command(['git', 'diff', '--check', common])
    paths = subprocess.check_output(['git', 'ls-files', '-o', '--exclude-standard', '-z'], cwd=ROOT).decode().split('\0')
    for path in paths:
        if path and (ROOT / path).is_file():
            result = subprocess.run(['git', 'diff', '--no-index', '--check', '/dev/null', path], cwd=ROOT, capture_output=True, text=True)
            if result.returncode > 1 or result.stdout or result.stderr:
                print(result.stdout + result.stderr, end='', flush=True)
                raise subprocess.CalledProcessError(result.returncode, ['git', 'diff', '--check', path])


def fast(catalog):
    tags = ','.join(['batz_app', *[tag(p) for p in sorted(catalog)]])
    run_jobs([
        (0, lambda: command(['go', 'test', './...', '-p', '1', '-count=1'])),
        (0, lambda: command(['go', 'vet', '-p', '1', '-tags', tags, './...'])),
        (0, lambda: command([sys.executable, '-I', '-m', 'unittest', 'discover', '-s', 'runtime/sqlmesh', '-p', 'test_inspect_preview.py'])),
        (0, lambda: command([sys.executable, '-I', '-m', 'unittest', 'discover', '-s', 'ci', '-p', 'test_*.py'])),
        (0, whitespace),
    ])


def app():
    # Exactly two package workers; the host allocator reserves both native slots.
    command(['go', 'test', '-tags', 'batz_app', './internal/app', './internal/run',
             './internal/scratch', '-run', '^TestApp', '-p', '2', '-count=1'])
    command([sys.executable, '-I', '-m', 'unittest', 'discover', '-s', 'runtime/sqlmesh', '-p', 'test_runner.py'])


def models(identity, item):
    project = ROOT / reference(item['path'], item['document']['normalization']['project'])
    if not list((project / 'tests').glob('test*.y*ml')):
        raise ValueError('missing requested native model tests: ' + identity)
    command([sys.executable, '-I', ROOT / 'runtime/sqlmesh/test_models.py', ROOT / item['path']])


def e2e(identity):
    source = ROOT / 'internal/app' / (identity.replace('-', '_') + '_e2e_test.go')
    if not source.is_file() or ('//go:build ' + tag(identity)) not in source.read_text():
        raise ValueError('missing requested E2E tests: ' + identity)
    command(['go', 'test', '-tags', tag(identity), './internal/app', '-run', '^TestE2E', '-p', '1', '-count=1'])


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('lane', choices=['fast', 'app', 'models', 'e2e', 'all-local'])
    parser.add_argument('pipelines', nargs='*')
    args = parser.parse_args()
    catalog = working_catalog()
    if args.pipelines and args.lane not in ('models', 'e2e'):
        raise ValueError('pipeline arguments require models or e2e lane')
    for identity in args.pipelines:
        if identity not in catalog:
            raise ValueError('unknown requested pipeline: ' + identity)
    selected = args.pipelines or sorted(catalog)
    lock = environment()
    if args.lane == 'fast':
        fast(catalog)
        return 0
    with prepare_native(lock):
        return native_jobs(args, selected, catalog)


def native_jobs(args, selected, catalog):
    jobs = []
    if args.lane == 'all-local':
        jobs.append((0, lambda: fast(catalog)))
    if args.lane in ('app', 'all-local'):
        jobs.append((2, app))
    if args.lane in ('models', 'all-local'):
        for identity in selected:
            jobs.append((1, lambda identity=identity: models(identity, catalog[identity])))
    if args.lane in ('e2e', 'all-local'):
        for identity in selected:
            jobs.append((1, lambda identity=identity: e2e(identity)))
    run_jobs(jobs)
    return 0


if __name__ == '__main__':
    started = time.monotonic()
    try:
        raise SystemExit(main())
    except (ValueError, KeyError) as error:
        print(str(error), file=sys.stderr)
        raise SystemExit(2)
    except FileNotFoundError as error:
        print('missing requested tool: ' + str(error), file=sys.stderr)
        raise SystemExit(3)
    except subprocess.CalledProcessError as error:
        raise SystemExit(error.returncode if error.returncode > 0 else 1)

    finally:
        print(json.dumps({"kind": "lane", "seconds": round(time.monotonic() - started, 3)}), flush=True)
