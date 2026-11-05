#!/usr/bin/env python3
"""Run a pipeline's actual adapter and native SQLMesh tests in private scratch."""
from collections import Counter
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
from pipelines import reference, validate_manifest


sys.path.insert(0, str(Path(__file__).resolve().parent))
from catalog import load_context, snapshot, configure_private, execution_scope, model_sources
from runner import validate_contract, check_build, verify_published, quote, ident
from inspect_preview import reviewed_expectations, check_reviewed_values

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
    return manifest, external, parsed, example, declared

def manifest_contract(manifest, route):
    return dict(manifest['normalization'], apiVersion='batz-execution/v0alpha1', pipeline=manifest['name'],
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



def pipeline_contract(context, manifest, parsed, example, declared, scratch, evaluated):
    """Actual adapter to declared native roots, with production validation only.

    The private engine already belongs to this selected models batch. No Batz
    workspace, inventory, publication or separate reader is created.
    """
    contract = manifest_contract(manifest, manifest['routes'][example['route']])
    selected = validate_contract(context, contract)
    connection = context.engine_adapter.connection
    connection.execute('CREATE SCHEMA IF NOT EXISTS batz_input')
    connection.execute("CREATE OR REPLACE VIEW batz_input.parsed AS SELECT *, 'revision' AS parsed_revision, 'file' AS parsed_file, file_row_number::BIGINT AS parsed_row FROM read_parquet("+quote(parsed)+", file_row_number=true)")
    request = dict(contract=contract, input_revision='revision', input_digest=hashlib.sha256(parsed.read_bytes()).hexdigest(),
                   attempt_id=scratch.name, evaluation_nonce=scratch.name)
    connection.execute('CREATE OR REPLACE TABLE batz_input.build_context AS SELECT '+
                       ','.join(quote(request[k])+' AS '+ident(k) for k in
                                ('input_revision','input_digest','attempt_id','evaluation_nonce')))
    expected = Counter(connection.execute('SELECT observation_id,parsed_revision,parsed_file,parsed_row,payload FROM batz_input.parsed').fetchall())
    # Refresh shared input before new owner roots/audits can observe it. The
    # pinned API must include local additions during the same restatement plan.
    refresh = {'restate_models':selected, 'always_include_local_changes':True} if evaluated.intersection(selected) else {}
    plan = context.plan_builder(skip_tests=True, execution_time='2026-01-03',
                                select_models=selected, backfill_models=selected, **refresh).build()
    context.apply(plan)
    evaluated.update(selected)
    adapter_db = context.engine_adapter
    proof = check_build(request, sum(expected.values()), adapter_db.fetchall(
        'SELECT input_revision,input_digest,attempt_id,evaluation_nonce,observations,mapped,unsupported,rejected FROM '+context.table_name(contract['evidence'])),
        adapter_db.fetchall('SELECT assertion_failures,capability_failures FROM '+context.table_name(contract['checks'])))
    binding = 'batz_contract'
    connection.execute('CREATE SCHEMA IF NOT EXISTS '+binding)
    for name, model in [('source_staging',contract['staging']), *contract['outputs'].items()]:
        connection.execute('CREATE OR REPLACE VIEW '+binding+'.'+ident(name)+' AS SELECT * FROM '+context.table_name(model))
    request['reject_path'] = str(scratch/'rejects.parquet')
    connection.execute('COPY (SELECT * FROM '+binding+".source_staging WHERE mapping_status='rejected') TO "+quote(request['reject_path'])+' (FORMAT PARQUET)')
    verify_published(connection, binding, request, expected, proof)
    probes = {}
    for name in contract['outputs']:
        table = binding+'.'+ident(name)
        probes[name] = connection.execute('SELECT typeof("@timestamp"),epoch_ns("@timestamp") FROM '+table).fetchall()
    if example.get('expectations'):
        expected_values = reviewed_expectations(ROOT/reference(declared,example['expectations']), contract['outputs'])
        table = binding+'.'+ident(expected_values['dataset'])
        records = [json.loads(row[0]) for row in connection.execute('SELECT to_json(t) FROM '+table+' t').fetchall()]
        types = connection.execute('SELECT * FROM '+table+' LIMIT 0').description
        for record in records:
            for column, native, *_ in types:
                if native.id.startswith('timestamp') and record[column] is not None:
                    record[column] = record[column].replace(' ', 'T', 1)
        check_reviewed_values(expected_values, records, len(records))
    print(json.dumps({'pipeline_contract':manifest['name'], 'example':example['id'],
                      'models':selected, 'observations':sum(expected.values()),
                      'native_time_probes':probes, 'checks_passed':True}),flush=True)


def main(manifest_paths, mode='both', all_tests=False):
    manifests = [validate_manifest(yaml.load(Path(path).read_text()),
                    Path(path).resolve().relative_to(ROOT.resolve()).as_posix()) for path in manifest_paths]
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
                evaluated = set()
                for index,(manifest,external,parsed,example,declared) in enumerate(proofs):
                    pipeline_contract(context, manifest, parsed, example, declared, scratch/('adapter-'+str(index)), evaluated)
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
