#!/usr/bin/env python3
"""Select affected contracts from merge-base and head manifest ownership."""
import json
import os
import tempfile
from contextlib import ExitStack, redirect_stdout
from pathlib import Path
import subprocess
import sys

sys.path.insert(0, str(Path(__file__).resolve().parent))
from pipelines import documents, reference
sys.path.insert(0, str(Path(__file__).resolve().parents[1]/"runtime/sqlmesh"))
from catalog import load_context, admitted_files, configure_private, model_sources, execution_scope, impacted_models, UNSUPPORTED


def git(root, *args):
    return subprocess.check_output(['git', *args], cwd=root)


def snapshot(root, revision, destination):
    entries = git(root, 'ls-tree', '-r', '-z', revision).split(b'\0')
    blobs = {}
    for entry in entries:
        if not entry:
            continue
        metadata, name = entry.split(b'\t', 1)
        path = name.decode()
        first = path.split('/')[0]
        native = first in {'models','tests','audits','macros','seeds','config.yaml'} | UNSUPPORTED
        if native and metadata.split()[0] != b'100644' and metadata.split()[0] != b'100755':
            raise ValueError('unsupported native entry: ' + path)
        if native or path.endswith(('.yaml','.yml')):
            blobs[path] = (metadata.split()[2], native)
    result = subprocess.run(['git', 'cat-file', '--batch'], cwd=root,
                            input=b'\n'.join(v[0] for v in blobs.values()) + b'\n', capture_output=True, check=True)
    values = {}
    offset = 0
    for name, (_, native) in blobs.items():
        end = result.stdout.index(b'\n', offset)
        size = int(result.stdout[offset:end].split()[2])
        offset = end + 1
        data = result.stdout[offset:offset+size]
        offset += size + 1
        if name.endswith(('.yaml','.yml')):
            values[name] = data.decode()
        if native:
            path = destination / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(data)
    return documents(values, values.__getitem__, versions=('batz/v0alpha1','batz/v0alpha2'))


def contracts(item):
    value = item['document']
    for route in value['routes'].values():
        yield dict(value['normalization'], pipeline=value['name'],
                   outputs=route['output']['normalized']['models'])


def native_context(project, catalog):
    if any(reference(item['path'], item['document']['normalization']['project']) != '.' for item in catalog.values()):
        raise ValueError('unsupported native project layout')
    admitted_files(project, tests=True)
    configure_private(project, project.parent/'state')
    context = load_context("discovery", paths=[project], load_state=False)
    try:
        scopes = {owner: set().union(*(set(execution_scope(context,c,owner)) for c in contracts(item)))
                  for owner,item in catalog.items()}
        return context, scopes
    except BaseException:
        context.close()
        raise


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
        owned = {manifest}
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
    changed = changed_paths(root, common, head)
    reasons = []
    with tempfile.TemporaryDirectory(prefix='batz-impact-') as scratch, ExitStack() as stack:
        scratch = Path(scratch)
        os.environ['SQLMESH_HOME'] = str(scratch/'home')
        os.environ['SQLMESH__DISABLE_ANONYMIZED_ANALYTICS'] = 'true'
        before = snapshot(root, common, scratch/'base/project')
        after = snapshot(root, head, scratch/'head/project')
        owned = ownership(before)
        for identity, paths in ownership(after).items():
            owned.setdefault(identity,set()).update(paths)
        affected = set()
        test_affected = set()
        broaden_tests = False
        shared_app = False
        docs = False
        broaden = False
        native_changes = set()
        executable_changes = set(changed)
        bridge = {'internal/app/explain.go','internal/app/pipeline.go','internal/app/native.go',
                  'internal/app/preview.go','internal/app/dependencies.go','internal/run/collection.go',
                  'internal/run/normalization.go','internal/run/child.go',
                  'internal/app/native_test.go','internal/app/publication_test.go',
                  'internal/app/dependencies_test.go','internal/app/shared_reader_test.go'}
        for path in sorted(changed):
            owners = {owner for owner,paths in owned.items()
                      if any(path == asset or path.startswith(asset+'/') for asset in paths)}
            if path.endswith(('.md','.rst')) and not any(path in owned[owner] for owner in owners):
                docs = True
                executable_changes.remove(path)
                continue
            affected.update(owners)
            if owners:
                reasons.append('asset '+path+': '+','.join(sorted(owners)))
            if any('/testdata/' in (after.get(owner) or before[owner])['path'] for owner in owners):
                shared_app = True
            first = path.split('/')[0]
            if first in {'models','tests'}:
                native_changes.add(path)
            elif path in bridge or (not owners and not path.startswith(('internal/app/','internal/run/','internal/config/','cmd/batz/'))):
                broaden = True
                reasons.append('global or unattributed input: '+path)
            elif not owners:
                shared_app = True
                reasons.append('shared application: '+path)
        removed = sorted(set(before)-set(after))
        if removed:
            broaden = True
            reasons.append('removed pipeline contracts: '+','.join(removed))
        # Docs-only remains independent of SQLMesh import and graph setup.
        if affected or shared_app or broaden or native_changes:
            # Logs and caches belong to private scratch, not the checkout.
            previous = Path.cwd()
            os.chdir(scratch)
            stack.callback(os.chdir, previous)
            with redirect_stdout(sys.stderr):
                head_context, head_scopes = native_context(scratch/'head/project',after)
                stack.callback(head_context.close)
                try:
                    base_context, base_scopes = native_context(scratch/'base/project',before)
                except Exception as error:
                    broaden = True
                    reasons.append('unsupported-base: '+str(error))
                else:
                    stack.callback(base_context.close)
                    impact = impacted_models(base_context,head_context,native_changes)
                    for owner,item in {**before, **after}.items():
                        if '/testdata/' in item['path'] and impact & (base_scopes.get(owner,set()) | head_scopes.get(owner,set())):
                            shared_app = True
                            reasons.append('shared runtime model impact: '+owner)
                    # Test coverage includes owned helpers outside runtime roots.
                    for context, scopes in ((base_context,base_scopes),(head_context,head_scopes)):
                        for name,path in model_sources(context).items():
                            owner = Path(path).parts[1]
                            if owner in scopes:
                                scopes[owner].add(name)
                        for scope in scopes.values():
                            for name in tuple(scope):
                                scope.update(context.dag.upstream(name))
                    covered = set().union(*base_scopes.values(),*head_scopes.values())
                    if impact - covered:
                        broaden_tests = True
                        reasons.append('unconsumed native model: ' + ','.join(sorted(impact-covered)))
                    for owner in set(before)|set(after):
                        if impact & (base_scopes.get(owner,set())|head_scopes.get(owner,set())):
                            affected.add(owner)
                            reasons.append('native model impact: '+owner)
                    attributed = set(model_sources(base_context).values()) | set(model_sources(head_context).values())
                    for context, scopes in ((base_context,base_scopes),(head_context,head_scopes)):
                        for metadata in context.select_tests():
                            path = metadata.path.relative_to(context.path).as_posix()
                            if path not in native_changes:
                                continue
                            attributed.add(path)
                            model = context.get_model(metadata.model_name,raise_if_missing=True).fqn
                            consumers = {owner for owner,scope in scopes.items() if model in scope}
                            if not consumers:
                                broaden_tests = True
                                reasons.append('unconsumed native test: '+metadata.test_name)
                            for owner in consumers:
                                test_affected.add(owner)
                                reasons.append('native test '+metadata.test_name+': '+owner)
                    for path in native_changes - attributed:
                        broaden = True
                        reasons.append('unattributed native input: '+path)
        adapters = sorted(affected & set(after))
        selected = sorted((affected | test_affected) & set(after))
        if broaden_tests:
            selected = sorted(after)
        if broaden:
            selected = sorted(after)
            adapters = selected
            shared_app = True
        native_tests_only = bool(executable_changes) and executable_changes == native_changes and all(
            path.startswith('tests/') and path.endswith(('.yaml','.yml')) for path in executable_changes)
        return {'base':common,'head':git(root,'rev-parse',head).decode().strip(),
                'fast':(bool(selected) and not native_tests_only) or bool(adapters) or shared_app or broaden,'app':shared_app,'models':selected,'adapters':adapters,
                'docs':docs,'removed_pipelines':removed,'reasons':sorted(set(reasons))}


if __name__ == '__main__':
    try:
        print(json.dumps(select_changes(Path.cwd(), sys.argv[1], sys.argv[2] if len(sys.argv)>2 else 'HEAD'), sort_keys=True))
    except (ValueError, KeyError, IndexError, subprocess.CalledProcessError) as error:
        print('selection failed: ' + str(error), file=sys.stderr)
        raise SystemExit(2)
