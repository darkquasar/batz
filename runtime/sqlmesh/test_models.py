#!/usr/bin/env python3
"""Run a pipeline's actual adapter and native SQLMesh tests in private scratch."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import duckdb
from ruamel.yaml import YAML

yaml = YAML(typ="safe")

ROOT = Path(__file__).resolve().parents[2]


sys.path.insert(0, str(ROOT / 'ci'))
from pipelines import reference


def main(manifest_path):
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
    with tempfile.TemporaryDirectory(prefix='batz-models-') as scratch:
        scratch = Path(scratch)
        os.environ['SQLMESH_HOME'] = str(scratch / 'sqlmesh-home')
        os.environ['SQLMESH__DISABLE_ANONYMIZED_ANALYTICS'] = 'true'
        from sqlmesh import Context
        project = scratch / 'project'
        shutil.copytree(resolve(manifest['normalization']['project']), project)
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
        (project / 'external_models.yaml').write_text(json.dumps(external))
        connection.close()
        config = yaml.load((project / 'config.yaml').read_text())
        gateway = config['gateways']['local']
        gateway['connection']['database'] = str(scratch / 'working.duckdb')
        gateway['state_connection']['database'] = str(scratch / 'state.duckdb')
        (project / 'config.yaml').write_text(json.dumps(config))
        os.environ['SQLMESH__DISABLE_ANONYMIZED_ANALYTICS'] = 'true'
        os.chdir(scratch)
        context = Context(paths=[str(project)])
        try:
            result = context.test(stream=sys.stdout, verbosity=2)
            if result.testsRun == 0 or not result.wasSuccessful() or result.skipped:
                print('Native model result:', result.testsRun, 'tests;', result.failures, result.errors, file=sys.stderr)
                raise SystemExit(1)
        finally:
            context.close()


class SelectedPipeline(unittest.TestCase):
    def test_actual_adapter_and_native_model_contracts(self):
        main(MANIFEST)


if __name__ == '__main__':
    MANIFEST = sys.argv[1]
    unittest.main(argv=[sys.argv[0]], verbosity=2)
