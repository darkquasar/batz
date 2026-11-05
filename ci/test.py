#!/usr/bin/env python3
"""Shared local/CI commands around Go, unittest and native SQLMesh."""
import argparse
from concurrent.futures import ThreadPoolExecutor
from contextlib import contextmanager, nullcontext
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
from pipelines import ROOT, reference, working_catalog


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
    tags = 'batz_app'
    run_jobs([
        (0, lambda: command(['go', 'test', './...', '-p', '1', '-count=1'])),
        (0, lambda: command(['go', 'vet', '-p', '1', '-tags', tags, './...'])),
        (0, lambda: command([sys.executable, '-I', '-m', 'unittest', 'discover', '-s', 'runtime/sqlmesh', '-p', 'test_inspect_preview.py'])),
        (0, lambda: command([sys.executable, '-I', '-m', 'unittest', 'discover', '-s', 'ci', '-p', 'test_*.py'])),
        (0, whitespace),
    ])


def app():
    def python_contracts():
        # One interpreter avoids repeated SQLMesh imports; each fixture owns state.
        command([sys.executable, '-I', '-c',
                 "import sys,unittest,os,tempfile; scratch=tempfile.TemporaryDirectory(prefix='batz-contracts-'); "
                 "os.environ['SQLMESH_HOME']=scratch.name; os.environ['SQLMESH__DISABLE_ANONYMIZED_ANALYTICS']='true'; sys.path[:0]=['runtime/sqlmesh','ci']; "
                 "suite=unittest.TestLoader().loadTestsFromNames(['test_runner','test_scope','native_select_test']); "
                 "result=unittest.TextTestRunner(verbosity=1).run(suite); sys.exit(not result.wasSuccessful() or bool(result.skipped))"])
    run_jobs([
        (1, lambda: go_tests(['go','test','-json','-tags','batz_app','./internal/app','-run','^TestApp','-p','1','-count=1'], 'TestApp', required={'TestAppSharedPipelineJourney'})),
        (1, lambda: go_tests(['go','test','-json','-tags','batz_app','./internal/run','./internal/scratch','-run','^TestApp','-p','1','-count=1'], 'TestApp')),
        (1, python_contracts),
    ])


def models(selected, catalog, mode='both'):
    command([sys.executable, '-I', ROOT/'runtime/sqlmesh/test_models.py', '--mode', mode,
             *(['--all-tests'] if set(selected) == set(catalog) else []),
             *[ROOT/catalog[identity]['path'] for identity in sorted(set(selected))]])


def go_tests(argv, prefix, required=()):
    """Require executed, non-skipped tests in every requested Go package."""
    started = time.monotonic()
    print('+ ' + ' '.join(argv), flush=True)
    ran, passed, skipped = set(), set(), set()
    packages, package_pass = set(), set()
    with subprocess.Popen(argv, cwd=ROOT, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
                          text=True, pass_fds=getattr(_worker, 'fds', ())) as process:
        for line in process.stdout:
            try:
                event = json.loads(line)
            except ValueError:
                print(line, end='', flush=True)
                continue
            print(event.get('Output', ''), end='', flush=True)
            name, action = event.get('Test', ''), event.get('Action')
            package = event.get('Package', '')
            if action == 'start':
                packages.add(package)
            if name.startswith(prefix):
                name = (package, name)
                if action == 'run':
                    ran.add(name)
                elif action == 'pass':
                    passed.add(name)
                elif action == 'skip':
                    skipped.add(name)
            elif not name and action == 'pass':
                package_pass.add(package)
        code = process.wait()
    print(json.dumps({'kind': 'test', 'command': argv, 'exit': code,
                      'seconds': round(time.monotonic()-started, 3),
                      'executed': sorted(ran), 'passed': sorted(passed), 'skipped': sorted(skipped)}), flush=True)
    if code:
        raise subprocess.CalledProcessError(code, argv)
    if skipped:
        raise ValueError('selected '+prefix+' tests skipped: ' + str(sorted(skipped)))
    if not any('/' not in name for package,name in ran & passed) or ran != passed:
        raise ValueError('missing executed passing '+prefix+' tests')
    if not set(required) <= {name for package,name in passed}:
        raise ValueError('missing required shared journey: '+str(sorted(required)))
    if not package_pass or not packages <= package_pass:
        raise ValueError('missing passing '+prefix+' package result')
    if any(not any(p == package and '/' not in name for p,name in ran & passed) for package in packages):
        raise ValueError('missing executed passing '+prefix+' tests in requested package')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('lane', choices=['fast', 'app', 'models', 'all-local'])
    parser.add_argument('pipelines', nargs='*')
    parser.add_argument('--mode', choices=['both','native','adapters'],default='both')
    args = parser.parse_intermixed_args()
    catalog = working_catalog()
    if args.pipelines and args.lane != 'models':
        raise ValueError('pipeline arguments require models lane')
    for identity in args.pipelines:
        if identity not in catalog:
            raise ValueError('unknown requested pipeline: ' + identity)
    if args.mode != 'both' and args.lane != 'models':
        raise ValueError('--mode requires models lane')
    selected = args.pipelines or sorted(catalog)
    lock = environment()
    if args.lane == 'fast':
        fast(catalog)
        return 0
    with (nullcontext() if args.lane == 'models' and args.mode == 'native' else prepare_native(lock)):
        return native_jobs(args, selected, catalog)


def native_jobs(args, selected, catalog):
    jobs = []
    if args.lane == 'all-local':
        jobs.append((0, lambda: fast(catalog)))
    if args.lane in ('app', 'all-local'):
        jobs.append((0, app))
    if args.lane in ('models', 'all-local'):
        mode = args.mode
        jobs.append((1, lambda: models(selected,catalog,mode)))
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
