#!/usr/bin/env python3
"""Preparation-only side-by-side upgrade proof; never part of offline setup."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile


def require(condition, message):
    if not condition:
        raise ValueError(message)


def inventory(root):
    content = json.loads((root / 'release-content.json').read_bytes())
    require(all(hashlib.sha256((root / name).read_bytes()).hexdigest() == expected
                for name, expected in content['files'].items()), 'changed installation content')
    return content


def snapshot(root):
    result = {}
    for path in [root, *sorted(root.rglob('*'))]:
        stat = path.lstat()
        value = [stat.st_mode, stat.st_mtime_ns]
        if path.is_symlink():
            value.append(os.readlink(path))
        elif path.is_file():
            value.append(hashlib.sha256(path.read_bytes()).hexdigest())
        result[str(path.relative_to(root))] = value
    return result


def verify(first, archive, mise):
    first = first.resolve()
    content_a = inventory(first)
    require(content_a['development'] is False and content_a['dirty'] is False,
            'upgrade proof requires an official candidate A')
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(('PYTHON', 'UV_', 'PIP_', 'CONDA_')) and key != 'VIRTUAL_ENV'}
    env.update(MISE_BIN=str(mise), PYTHONDONTWRITEBYTECODE='1')
    with tempfile.TemporaryDirectory(prefix='Batz upgrade proof ') as base:
        base = Path(base)
        caller = base / 'unrelated caller'; caller.mkdir()

        def run(argv, cwd=caller, extra=None, failure=None):
            process = subprocess.run(list(map(str, argv)), cwd=cwd, env=dict(env, **(extra or {})),
                                     text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, timeout=1200)
            print(json.dumps({'argv': list(map(str, argv)), 'cwd': str(cwd), 'exit': process.returncode}), flush=True)
            print(process.stdout, flush=True)
            if failure:
                require(process.returncode != 0 and failure in process.stdout, 'wrong upgrade failure')
            else:
                require(process.returncode == 0, 'upgrade proof command failed')
            return process.stdout

        def identity(root, content):
            actual = json.loads(run([root / 'bin/batz', 'version', '--format', 'json']))
            require(all(actual[key] == content[key] for key in ('version', 'source_sha', 'target', 'development', 'dirty')),
                    'CLI/content identity mismatch')

        def python(root, *args):
            return [root / 'runtime/.local/venv/bin/python', '-I', '-B', *args]

        def execute(root, workspace):
            result = json.loads(run([root / 'bin/batz', 'run', root / 'pipelines/aws-cloudtrail/pipeline.yaml',
                                     '--workspace', workspace, '--binding-id', 'example', '--route', 'local_raw',
                                     '--input', 'path=' + str(root / 'pipelines/aws-cloudtrail/fixtures/wiring.json')]))
            require(result['exit'] == 0 and result['catalog'], 'upgrade run did not publish')
            return result['catalog']

        def reader(root, catalog):
            run(python(root, '-c', "import duckdb,sys; c=duckdb.connect(); c.execute(\"ATTACH '\" + sys.argv[1].replace(\"'\", \"''\") + \"' AS published (READ_ONLY)\"); assert c.sql('SELECT count(*) FROM published.example.api_activity').fetchone()[0] > 0", catalog))

        # The producer has already inspected this development-only archive.
        with tarfile.open(archive, 'r:gz') as opened:
            opened.extractall(base, filter='data')
        second = base / archive.name.removesuffix('.tar.gz')
        content_b = inventory(second)
        require(content_b['development'] is True and content_b['dirty'] is False and
                content_a['version'] != content_b['version'], 'B must be a distinct clean development fixture')
        require(all(content_a[key] == content_b[key] for key in ('source_sha', 'source_tree_sha256', 'target')),
                'upgrade fixture source/target mismatch')
        identity(first, content_a); identity(second, content_b)
        workspace = base / 'original workspace'
        catalog_a = execute(first, workspace); reader(first, catalog_a)
        receipt = (first / 'runtime/.local/installation.json').read_bytes()
        old_environment = snapshot(first / 'runtime/.local/venv')
        old_evidence = snapshot(workspace)

        def unchanged():
            require(receipt == (first / 'runtime/.local/installation.json').read_bytes(), 'A receipt changed')
            require(old_environment == snapshot(first / 'runtime/.local/venv'), 'A environment changed')
            require(old_evidence == snapshot(workspace), 'A evidence changed')
            inventory(first)
            identity(first, content_a); reader(first, catalog_a)

        run([mise, 'trust'], second)
        lock = second / 'uv.lock'; original = lock.read_bytes()
        lock.write_bytes(original + b'\n# deliberate upgrade lock mismatch\n')
        try:
            run([mise, 'install'], second,
                {'VIRTUAL_ENV': str(first / 'runtime/.local/venv'),
                 'UV_PROJECT_ENVIRONMENT': str(first / 'runtime/.local/venv')},
                failure='lock file checksum mismatch: uv.lock')
        finally:
            lock.write_bytes(original)
        unchanged()
        run([mise, 'install'], second)
        identity(second, content_b)
        prefix = ('-c', 'import sys; print(sys.prefix)')
        require(run(python(first, *prefix)).strip() != run(python(second, *prefix)).strip(),
                'upgrade reused the old environment')
        catalog_b = execute(second, base / 'upgraded workspace')
        reader(second, catalog_b); reader(second, catalog_a)
        unchanged(); inventory(second)
        return {'target': content_a['target'], 'source_sha': content_a['source_sha'],
                'first_version': content_a['version'], 'upgrade_version': content_b['version'],
                'upgrade_fixture_development_only': True, 'outcome': 'success'}


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--first', type=Path, required=True)
    parser.add_argument('--archive', type=Path, required=True)
    parser.add_argument('--mise', type=Path, required=True)
    args = parser.parse_args()
    print(json.dumps(verify(args.first, args.archive, args.mise), sort_keys=True), flush=True)
