#!/usr/bin/env python3
"""Select affected contracts from merge-base and head manifest ownership."""
import json
from pathlib import Path
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from pipelines import documents, reference


def git(root, *args):
    return subprocess.check_output(['git', *args], cwd=root)


def snapshot(root, revision):
    entries = git(root, 'ls-tree', '-r', '-z', revision).split(b'\0')
    blobs = {}
    for entry in entries:
        if not entry:
            continue
        metadata, name = entry.split(b'\t', 1)
        if name.endswith((b'.yaml', b'.yml')):
            blobs[name.decode()] = metadata.split()[2]
    # One standard Git batch reads all candidate manifests; no process per file.
    result = subprocess.run(['git', 'cat-file', '--batch'], cwd=root,
                            input=b'\n'.join(blobs.values()) + b'\n', capture_output=True, check=True)
    values = {}
    offset = 0
    for name in blobs:
        end = result.stdout.index(b'\n', offset)
        size = int(result.stdout[offset:end].split()[2])
        offset = end + 1
        values[name] = result.stdout[offset:offset+size].decode()
        offset += size + 1
    # Older declarations supply ownership only, never execution compatibility.
    return documents(values, values.__getitem__, versions=('batz/v0alpha1', 'batz/v0alpha2'))


def changed_paths(root, base, head):
    values = git(root, 'diff', '--name-status', '--find-renames', '-z', base, head).decode().split('\0')
    result = set()
    index = 0
    while index < len(values) and values[index]:
        status = values[index]
        count = 2 if status[0] in ('R', 'C') else 1
        result.update(values[index+1:index+1+count])
        index += count + 1
    return result


def ownership(catalog):
    paths = {}
    for identity, item in catalog.items():
        manifest, value = item['path'], item['document']
        owned = {manifest, reference(manifest, value['normalization']['project']), 'internal/app/' + identity.replace('-', '_') + '_e2e_test.go'}
        if '/testdata/' in manifest:
            owned.add(str(Path(manifest).parent))
        for route in value['routes'].values():
            owned.add(reference(manifest, route['input']['config']))
        for example in value.get('explain', {}).get('examples', []):
            owned.update(reference(manifest, example[key]) for key in ('input', 'expectations') if example.get(key))
        paths[identity] = owned
    return paths


def select_changes(root, base, head):
    common = git(root, 'merge-base', base, head).decode().strip()
    before, after = snapshot(root, common), snapshot(root, head)
    owned = ownership(before)
    for identity, paths in ownership(after).items():
        owned.setdefault(identity, set()).update(paths)
    affected = set()
    shared_app = False
    docs = False
    broaden = False
    bridge = {'internal/app/explain.go', 'internal/app/pipeline.go', 'internal/app/native.go', 'internal/app/preview.go',
              'internal/app/dependencies.go', 'internal/run/collection.go',
              'internal/run/normalization.go', 'internal/run/child.go'}
    for path in changed_paths(root, common, head):
        if path.endswith(('.md', '.rst')) and not any(any(path == owner or path.startswith(owner + '/') for owner in paths) for paths in owned.values()):
            docs = True
            continue
        owners = {identity for identity, paths in owned.items()
                  if any(path == owner or path.startswith(owner + '/') for owner in paths)}
        affected.update(owners)
        if any('/testdata/' in (after.get(owner) or before[owner])['path'] for owner in owners):
            shared_app = True
        if path in bridge:
            broaden = True
        elif not owners and path.startswith(('internal/app/', 'internal/run/', 'internal/config/', 'cmd/batz/')):
            shared_app = True
        elif not owners:
            broaden = True
    removed = sorted(set(before) - set(after))
    broaden = broaden or bool(removed)
    selected = sorted(affected & set(after))
    e2e = set(selected)
    if broaden:
        selected = sorted(after)
        # Fixed compatibility probes, not the entire growing pipeline catalog.
        production = sorted(identity for identity, item in after.items() if '/testdata/' not in item['path'])
        probes = sorted(identity for identity, item in after.items() if '/testdata/' in item['path'])
        e2e.update(production[:1] + probes[:1])
        shared_app = True
    return {'base': common, 'head': git(root, 'rev-parse', head).decode().strip(),
            'fast': bool(affected) or shared_app or broaden, 'app': shared_app, 'models': selected,
            'e2e': sorted(e2e), 'docs': docs, 'removed_pipelines': removed}


if __name__ == '__main__':
    try:
        print(json.dumps(select_changes(Path.cwd(), sys.argv[1], sys.argv[2] if len(sys.argv)>2 else 'HEAD'), sort_keys=True))
    except (ValueError, KeyError, IndexError, subprocess.CalledProcessError) as error:
        print('selection failed: ' + str(error), file=sys.stderr)
        raise SystemExit(2)
