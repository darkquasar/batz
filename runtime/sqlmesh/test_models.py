#!/usr/bin/env python3
"""Run a pipeline's actual adapter and native SQLMesh tests in private scratch."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

import duckdb
from ruamel.yaml import YAML

yaml = YAML(typ="safe")

ROOT = Path(__file__).resolve().parents[2]


sys.path.insert(0, str(ROOT / 'ci'))
from pipelines import reference


sys.path.insert(0, str(Path(__file__).resolve().parent))
from catalog import load_context, snapshot, configure_private, execution_scope, model_sources

def adapter(manifest_path, scratch):
    declared = Path(manifest_path).resolve().relative_to(ROOT).as_posix()
    def resolve(value):
        return ROOT / reference(declared, value)
    manifest = yaml.load(Path(manifest_path).read_text())
    # Fixtures are owned by the manifest/project, never a manual pipeline registry.
    examples = manifest.get('explain', {}).get('examples', [])
    if not examples:
        raise ValueError('native model verification requires a declared representative example')
    example = examples[0]
    route = manifest['routes'][example['route']]
    source = resolve(example['input'])
    source_data = source.read_bytes()
    if examples and hashlib.sha256(source_data).hexdigest() != examples[0]['sha256']:
        raise AssertionError('declared fixture checksum mismatch')
    stage = scratch / 'adapter'
    stage.mkdir()
    (stage / 'input.json').write_bytes(source_data)
    identity = {'binding_id': 'adapter', 'run_id': 'run', 'batch_id': 'batch', 'attempt_id': 'attempt', 'case_id': None}
    request = {'identity': identity, 'framing': route['input']['framing'],
               'input': {'id': 'fixture', 'locator': str(source), 'sha256': hashlib.sha256(source_data).hexdigest(), 'bytes': len(source_data)},
               'stage': str(stage), 'source': str(stage / 'input.json'), 'acquired_at': '2026-10-25T00:00:00Z',
               'limits': {'input_bytes': 16 << 20, 'records': 10000, 'collection_seconds': 30}}
    (stage / 'request.json').write_text(json.dumps(request))
    subprocess.run([str(Path(os.environ.get('BATZ_TEST_RUNTIME', ROOT / 'runtime/.local')) / 'batz-collector'), str(resolve(route['input']['config'])), str(stage / 'request.json')], check=True)
    receipt = json.loads((stage / 'receipt.json').read_text())
    if not receipt['complete'] or receipt['identity'] != identity:
        raise AssertionError('adapter completion identity')
    connection = duckdb.connect(config={'threads': 1})
    parsed = stage / receipt['parsed_files'][0]['path']
    rows = connection.execute('SELECT record_ordinal,payload FROM read_parquet(?) ORDER BY record_ordinal', [str(parsed)]).fetchall()
    expected = json.loads(source_data)['Records'] if route['input']['framing'] == 'records-json' else [json.loads(line) for line in source_data.splitlines() if line.strip()]
    if [(ordinal, json.loads(payload)) for ordinal, payload in rows] != list(enumerate(expected)):
        raise AssertionError('independent source-to-parsed fidelity')
    invalid_sources = [b'not JSON', b'{"Records":['] if route['input']['framing'] == 'records-json' else [b'{}\n{', b'{} x\n']
    for index, invalid in enumerate(invalid_sources):
        invalid_stage = scratch / ('invalid-' + str(index))
        invalid_stage.mkdir()
        invalid_request = dict(request, stage=str(invalid_stage), source=str(invalid_stage / 'input.json'))
        invalid_request['input'] = dict(request['input'], sha256=hashlib.sha256(invalid).hexdigest(), bytes=len(invalid))
        (invalid_stage / 'input.json').write_bytes(invalid)
        (invalid_stage / 'request.json').write_text(json.dumps(invalid_request))
        result = subprocess.run([str(Path(os.environ.get('BATZ_TEST_RUNTIME', ROOT / 'runtime/.local')) / 'batz-collector'), str(resolve(route['input']['config'])), str(invalid_stage / 'request.json')], capture_output=True)
        if result.returncode != 2:
            raise AssertionError(f'invalid framing exit={result.returncode}, expected 2: {result.stderr.decode()}')
        if (invalid_stage / 'receipt.json').exists():
            raise AssertionError('invalid framing acquired a completion receipt: ' + result.stderr.decode())
    # Derive native input types from actual adapter output, without a schema mirror.
    columns = {r[0]: r[1] for r in connection.execute('DESCRIBE SELECT *, \'revision\' AS parsed_revision, \'file\' AS parsed_file, file_row_number::BIGINT AS parsed_row FROM read_parquet(?, file_row_number=true)', [str(parsed)]).fetchall()}
    external = [{'name': 'batz_input.parsed', 'columns': columns},
                {'name': 'batz_input.build_context', 'columns': {k: 'VARCHAR' for k in ('input_revision', 'input_digest', 'attempt_id', 'evaluation_nonce')}}]

    connection.close()
    return manifest, external, parsed

def manifest_contract(manifest, route):
    return dict(manifest['normalization'], pipeline=manifest['name'],
                outputs=route['output']['normalized']['models'])


def run_selected_tests(context, selected):
    metadata = context.select_tests(model_names=selected)
    expected = {m.fully_qualified_test_name for m in metadata}
    if not expected:
        raise ValueError('missing requested native model tests')
    result = context.test(tests=sorted(expected), raise_on_unknown_paths=True, stream=sys.stdout, verbosity=2)
    executed = {str(test.path)+'::'+test.test_name for test in result.successes}
    print(json.dumps({'native_tests_selected':sorted(expected), 'native_tests_executed':sorted(executed)}),flush=True)
    if (result.testsRun != len(expected) or executed != expected or
            not result.wasSuccessful() or result.skipped):
        print(result.errors, result.failures, file=sys.stderr)
        raise ValueError('native model failure, skipped or omitted selected tests')
    return expected


def test_scope(context, owners, execution):
    selected = set(execution)
    selected.update(name for name,path in model_sources(context).items() if Path(path).parts[1] in owners)
    for name in tuple(selected):
        selected.update(context.dag.upstream(name))
    return selected


def main(manifest_paths, mode='both', all_tests=False):
    manifests = [yaml.load(Path(path).read_text()) for path in manifest_paths]
    if len({m['name'] for m in manifests}) != len(manifests):
        raise ValueError('duplicate requested pipeline')
    with tempfile.TemporaryDirectory(prefix='batz-models-') as directory:
        scratch = Path(directory)
        os.environ['SQLMESH_HOME'] = str(scratch/'sqlmesh-home')
        os.environ['SQLMESH__DISABLE_ANONYMIZED_ANALYTICS'] = 'true'
        project = snapshot(ROOT, scratch/'project', tests=mode!='adapters')
        configure_private(project,scratch/'state')
        proofs = []
        if mode != 'native':
            for index,path in enumerate(manifest_paths):
                private = scratch/('adapter-'+str(index))
                private.mkdir()
                proofs.append(adapter(path,private))
            external = proofs[0][1]
            if any(p[1] != external for p in proofs):
                raise ValueError('incompatible actual parsed schemas in selected adapters')
            (project/'external_models.yaml').write_text(json.dumps(external))
        previous = Path.cwd()
        os.chdir(scratch)
        try:
            context = load_context("model-tests" if mode != "adapters" else "adapter", paths=[project],load_state=False)
            try:
                scopes = {}
                for manifest in manifests:
                    scopes[manifest['name']] = test_scope(context, {manifest['name']}, set().union(*(set(execution_scope(context,manifest_contract(manifest,route),manifest['name'])) for route in manifest['routes'].values())))
                # The actual adapter output feeds its native staging query. No
                # duplicate full-project unit suite or mirrored input schema.
                for manifest,external,parsed in proofs:
                    adapter_db = context.engine_adapter
                    adapter_db.execute('CREATE SCHEMA IF NOT EXISTS batz_input')
                    escaped = str(parsed).replace("'", "''")
                    adapter_db.execute("CREATE OR REPLACE VIEW batz_input.parsed AS SELECT *, 'revision' AS parsed_revision, 'file' AS parsed_file, file_row_number::BIGINT AS parsed_row FROM read_parquet('"+escaped+"',file_row_number=true)")
                    rendered = context.render(manifest['normalization']['staging'], expand=True)
                    count = adapter_db.fetchone('SELECT count(*) FROM ('+rendered.sql(dialect='duckdb')+')')[0]
                    actual = adapter_db.fetchone('SELECT count(*) FROM batz_input.parsed')[0]
                    if count != actual:
                        raise ValueError('actual adapter/staging integration lost observations')
                    print(json.dumps({'adapter':manifest['name'],'actual_staging_rows':count}),flush=True)
                if mode != 'adapters':
                    for owner,scope in scopes.items():
                        if not context.select_tests(model_names=scope):
                            raise ValueError('missing requested native model tests: '+owner)
                    run_selected_tests(context,None if all_tests else set().union(*scopes.values()))
            finally:
                context.close()
        finally:
            os.chdir(previous)


if __name__ == '__main__':
    import argparse
    parser = argparse.ArgumentParser()
    parser.add_argument('--mode',choices=['both','native','adapters'],default='both')
    parser.add_argument('--all-tests',action='store_true')
    parser.add_argument('manifests',nargs='+')
    args = parser.parse_args()
    main(args.manifests,args.mode,args.all_tests)
