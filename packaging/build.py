#!/usr/bin/env python3
"""Build complete native Batz archives and assemble the immutable candidate set.

No downloads, dependency installation, tag creation or publication occur here.
"""
import argparse
import gzip
import hashlib
import io
import json
import os
import pathlib
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile

TARGETS = ('darwin/arm64', 'linux/amd64')
SCHEMA = 'batz-release-content/v0alpha1'
SETUP = ('mise.toml', 'mise.lock', 'pyproject.toml', 'uv.lock', 'ci/setup.py')
REQUIRED = SETUP + (
    'go.mod', 'go.sum', 'config.yaml', 'runtime/dependencies.lock.json', 'runtime/requirements.lock',
    'runtime/check-requirements.lock', 'runtime/sqlmesh/runner.py',
    'runtime/sqlmesh/catalog.py', 'runtime/sqlmesh/inspect_preview.py',
    'docs/usage/install.md', 'docs/usage/ci.md',
)
BINARY_PATHS = ('bin/batz', 'runtime/.local/batz-collector')
SEMVER = re.compile(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?\Z')


def digest(data):
    return hashlib.sha256(data).hexdigest()


def json_bytes(value):
    return (json.dumps(value, sort_keys=True, indent=2) + '\n').encode()


def run(command, root, env=None):
    return subprocess.check_output(command, cwd=root, env=env, text=True).strip()


def regular(path):
    if path.is_symlink() or not path.is_file():
        raise ValueError(f'missing package input: {path}')
    # Parent symlinks can otherwise smuggle ignored external inputs into a release.
    for parent in path.parents:
        if parent.is_symlink():
            raise ValueError(f'symlink package input: {path}')


def source_identity(root, source_sha, development):
    head = run(['git', 'rev-parse', 'HEAD'], root)
    if source_sha != head or not re.fullmatch('[0-9a-f]{40}', source_sha):
        raise ValueError('source_sha must be the exact full checked-out HEAD')
    dirty = bool(run(['git', 'status', '--porcelain=v1', '--untracked-files=all'], root))
    if dirty and not development:
        raise ValueError('official release requires clean matching source; use --development for local candidates')
    paths = subprocess.check_output(['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=root).split(b'\0')
    tree = hashlib.sha256()
    for raw in sorted(set(paths) - {b''}):
        path = root / os.fsdecode(raw)
        if not path.exists():
            tree.update(raw + b'\0<deleted>\0')
            continue
        regular(path)
        tree.update(raw + b'\0' + path.read_bytes() + b'\0')
    return {'source_sha': head, 'development': development, 'dirty': dirty, 'source_tree_sha256': tree.hexdigest()}


def content_paths(root):
    for name in REQUIRED:
        path = root / name
        if not path.is_file() or path.is_symlink():
            raise ValueError(f'missing package input: {name}')
    names = set(REQUIRED)
    # SQLMesh bridge also contains the minimum common profile implementation.
    for directory in ('models', 'pipelines', 'schemas', 'audits', 'seeds', 'macros'):
        base = root / directory
        if directory in ('models', 'pipelines', 'schemas') and not base.is_dir():
            raise ValueError(f'missing package input: {directory}')
        if not base.exists():
            continue
        for path in base.rglob('*'):
            if path.is_symlink():
                raise ValueError(f'symlink package input: {path.relative_to(root)}')
            relative = path.relative_to(base)
            if any(part.startswith('.') or part in ('__pycache__', 'tests', 'testdata') for part in relative.parts):
                continue
            if path.is_file() and path.suffix in ('.sql', '.yaml', '.yml', '.json', '.jsonl', '.csv', '.md', '.txt', '.parquet', '.gz'):
                names.add(path.relative_to(root).as_posix())
    # Ship only executable bridge modules, never source-only tests/caches.
    for path in (root / 'runtime/sqlmesh').glob('*.py'):
        if not path.name.startswith('test_'):
            names.add(path.relative_to(root).as_posix())
    for directory in ('docs/usage', 'docs/sources'):
        for path in (root / directory).glob('*.md'):
            names.add(path.relative_to(root).as_posix())
    if (root / 'runtime/pyproject.toml').is_file():
        names.add('runtime/pyproject.toml')
    for name in ('README.md', 'LICENSE', 'LICENSE.md', 'NOTICE', 'NOTICE.md'):
        if (root / name).is_file():
            names.add(name)
    admitted = set(os.fsdecode(name) for name in subprocess.check_output(
        ['git', 'ls-files', '-z', '--cached', '--others', '--exclude-standard'], cwd=root).split(b'\0') if name)
    for name in sorted(names):
        if name not in admitted:
            raise ValueError('package input outside admitted source: ' + name)
        regular(root / name)
    return sorted(names)


def validate_version(version):
    match = SEMVER.fullmatch(version)
    if not match or (match.group(4) and any(part.isdigit() and len(part) > 1 and part[0] == '0' for part in match.group(4).split('.'))):
        raise ValueError('version must be strict v-prefixed SemVer without build metadata')


def write_tar(stage, path, top):
    # Stable file order, ownership and timestamps; the inventory has no self-hash.
    stage = stage.resolve()
    with path.open('wb') as raw, gzip.GzipFile(filename='', fileobj=raw, mode='wb', mtime=0) as compressed:
        with tarfile.open(fileobj=compressed, mode='w') as archive:
            for item in sorted(stage.rglob('*')):
                if not item.is_file():
                    continue
                regular(item)
                data = item.read_bytes()
                info = tarfile.TarInfo(f'{top}/{item.relative_to(stage).as_posix()}')
                info.size = len(data)
                info.mode = 0o755 if item.relative_to(stage).as_posix() in BINARY_PATHS else 0o644
                archive.addfile(info, io.BytesIO(data))


def build_archive(args):
    root = pathlib.Path(args.root).resolve()
    paths = content_paths(root)  # Fail before compiling when setup has not joined.
    validate_version(args.version)
    if args.target not in TARGETS:
        raise ValueError('unsupported native target')
    identity = source_identity(root, args.source_sha, args.development)
    go = pathlib.Path(args.go).resolve()
    regular(go)
    lock = json.loads((root / 'runtime/dependencies.lock.json').read_bytes())
    expected_go = lock['go']['version']
    host = {('Darwin', 'arm64'): 'darwin/arm64', ('Linux', 'x86_64'): 'linux/amd64'}.get((platform.system(), platform.machine()))
    expected_compiler = lock['go'].get('binary_sha256', {}).get(host)
    if not expected_compiler or digest(go.read_bytes()) != expected_compiler:
        raise ValueError('compiler integrity mismatch with approved Go archive')
    inherited = {key: value for key, value in os.environ.items()
                 if not key.startswith('GO') or key in ('GOCACHE', 'GOMODCACHE', 'GOPATH')}
    env = dict(inherited, GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off', CGO_ENABLED='0', GOMAXPROCS='2',
               GOENV='off', GOWORK='off', GOFLAGS='', GOEXPERIMENT='', GOCACHEPROG='')
    env.pop('GOROOT', None)
    env['GOOS'], env['GOARCH'] = args.target.split('/')
    if run([str(go), 'env', 'GOVERSION'], root, env) != 'go' + expected_go:
        raise ValueError('Go compiler does not match runtime lock')
    run([str(go), 'mod', 'verify'], root, env)
    flags = ' '.join(f'-X batz/internal/buildinfo.{key}={value}' for key, value in {
        'Version': args.version, 'SourceSHA': args.source_sha, 'Target': args.target,
        'Dirty': str(identity['dirty']).lower(), 'Mode': 'development' if args.development else 'release',
    }.items())
    output = pathlib.Path(args.output).resolve()
    output.mkdir(parents=True, exist_ok=True)
    name = f"batz-{args.version}-{args.target.replace('/', '-')}"
    destination = output / (name + '.tar.gz')
    if destination.exists():
        raise ValueError(f'refusing to overwrite archive: {destination}')
    with tempfile.TemporaryDirectory(prefix='batz-package-') as temporary:
        stage = pathlib.Path(temporary).resolve() / 'content'
        stage.mkdir()
        for relative in paths:
            target = stage / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(root / relative, target)
        for binary, package in zip(BINARY_PATHS, ('./cmd/batz', './cmd/batz-collector')):
            target = stage / binary
            target.parent.mkdir(parents=True, exist_ok=True)
            run([str(go), 'build', '-mod=readonly', '-trimpath', '-buildvcs=false', '-ldflags', flags, '-o', str(target), package], root, env)
        notices = stage / 'notices'
        notices.mkdir()
        (notices / 'README.txt').write_text('Batz bundles its repo-built collector and the pinned modules listed here.\nThe SQLMesh bridge contains the versioned minimum common profile.\nPython dependencies are installed separately by the reviewed setup lock.\n')
        modules = run([str(go), 'list', '-mod=readonly', '-deps', '-f', '{{if .Module}}{{.Module.Path}}\t{{.Module.Version}}\t{{.Module.Dir}}{{end}}', './cmd/batz', './cmd/batz-collector'], root, env)
        provenance = []
        for row in sorted(set(modules.splitlines())):
            columns = row.split('\t')
            if len(columns) != 3 or columns[0] == 'batz':
                continue
            module, version, directory = columns
            provenance.append({'module': module, 'version': version})
            for path in sorted(pathlib.Path(directory).iterdir()):
                if path.is_file() and path.name.upper().startswith(('LICENSE', 'NOTICE', 'COPYING', 'COPYRIGHT')):
                    regular(path)
                    target = notices / module.replace('/', '_') / path.name
                    target.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(path, target)
        goroot = pathlib.Path(run([str(go), 'env', 'GOROOT'], root, env))
        shutil.copyfile(goroot / 'LICENSE', notices / 'Go-LICENSE')
        (notices / 'modules.json').write_bytes(json_bytes(provenance))
        inventory = {'schema': SCHEMA, 'version': args.version, 'target': args.target, **identity,
                     'files': {path.relative_to(stage).as_posix(): digest(path.read_bytes()) for path in sorted(stage.rglob('*')) if path.is_file()}}
        (stage / 'release-content.json').write_bytes(json_bytes(inventory))
        # A concurrent source change must not produce a falsely bound artifact.
        if source_identity(root, args.source_sha, args.development) != identity:
            raise ValueError('source changed during package build')
        pending = pathlib.Path(temporary) / destination.name
        write_tar(stage, pending, name)
        inspect_archive(pending)
        shutil.copyfile(pending, destination)
    print(destination)


def inspect_archive(path):
    """Validate complete extracted bytes and return the frozen candidate target row."""
    path = pathlib.Path(path)
    with tarfile.open(path, 'r:gz') as archive:
        members = archive.getmembers()
        files = {}
        top = None
        for member in members:
            name = pathlib.PurePosixPath(member.name)
            if member.name != name.as_posix() or name.is_absolute() or '..' in name.parts or len(name.parts) < 2:
                raise ValueError('invalid archive path')
            if top is None:
                top = name.parts[0]
            if top != name.parts[0] or not member.isfile():
                raise ValueError('archive must contain regular files in one installation root')
            relative = pathlib.PurePosixPath(*name.parts[1:]).as_posix()
            if relative in files:
                raise ValueError('duplicate archive path')
            files[relative] = archive.extractfile(member).read()
    if 'release-content.json' not in files:
        raise ValueError('archive has no release-content inventory')
    content_bytes = files.pop('release-content.json')
    content = json.loads(content_bytes)
    validate_version(content['version'])
    if content.get('schema') != SCHEMA or content.get('target') not in TARGETS:
        raise ValueError('unsupported release-content identity')
    for name in REQUIRED + BINARY_PATHS:
        if name not in files:
            raise ValueError(f'missing package input: {name}')
    if {name: digest(data) for name, data in files.items()} != content.get('files'):
        raise ValueError('archive release-content digest mismatch')
    if not any(name.startswith('models/') for name in files) or not any(name.startswith('pipelines/') for name in files) or not any(name.startswith('schemas/') for name in files):
        raise ValueError('archive is missing native content')
    if top != f"batz-{content['version']}-{content['target'].replace('/', '-')}":
        raise ValueError('archive root does not match release identity')
    return {'target': content['target'], 'archive': path.name, 'sha256': digest(path.read_bytes()),
            'bytes': path.stat().st_size, 'cli_sha256': content['files']['bin/batz'],
            'collector_sha256': content['files']['runtime/.local/batz-collector'],
            'content_sha256': digest(content_bytes),
            'runtime_lock_sha256': content['files']['runtime/dependencies.lock.json']}, content


def assemble_candidate(args):
    if not re.fullmatch('sha256:[0-9a-f]{64}', args.docker_digest):
        raise ValueError('a real immutable Docker sha256 digest is required')
    records = [inspect_archive(path) for path in args.archive]
    if len(records) != 2 or {row['target'] for row, _ in records} != set(TARGETS):
        raise ValueError('candidate requires exactly both native targets')
    keys = ('version', 'source_sha', 'development', 'dirty', 'source_tree_sha256')
    identity = {key: records[0][1][key] for key in keys}
    if any({key: content[key] for key in keys} != identity for _, content in records):
        raise ValueError('native archives have different source identities')
    if not re.fullmatch('[0-9a-f]{40}', identity['source_sha']) or not re.fullmatch('[0-9a-f]{64}', identity['source_tree_sha256']):
        raise ValueError('invalid source identity')
    if type(identity['development']) is not bool or type(identity['dirty']) is not bool or (identity['dirty'] and not identity['development']):
        raise ValueError('invalid development identity')
    candidate = {'schema': 'batz-release-candidate/v0alpha1', **identity,
                 'targets': sorted((row for row, _ in records), key=lambda row: row['target']),
                 'docker': {'target': 'linux/amd64', 'digest': args.docker_digest,
                            'version': identity['version'], 'source_sha': identity['source_sha']}}
    destination = pathlib.Path(args.output)
    # Exclusive creation preserves retry identity; callers verify existing bytes.
    with destination.open('xb') as output:
        output.write(json_bytes(candidate))
    print(destination)


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    archive = commands.add_parser('archive')
    for flag in ('root', 'output', 'version', 'source-sha', 'target', 'go'):
        archive.add_argument('--' + flag, required=True)
    archive.add_argument('--development', action='store_true')
    candidate = commands.add_parser('candidate')
    candidate.add_argument('--archive', action='append', required=True)
    candidate.add_argument('--docker-digest', required=True)
    candidate.add_argument('--output', required=True)
    args = parser.parse_args(argv)
    if args.command == 'archive':
        build_archive(args)
    else:
        assemble_candidate(args)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, OSError, KeyError, tarfile.TarError, subprocess.CalledProcessError) as error:
        print(f'batz packaging: {error}', file=sys.stderr)
        sys.exit(1)
