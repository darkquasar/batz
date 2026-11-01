"""Native catalog closure and source-ownership contracts, using invented SQL."""
import json
import os
from pathlib import Path
import tempfile
import unittest
import importlib.util

spec = importlib.util.spec_from_file_location('scope_bridge', Path(__file__).with_name('runner.py'))
bridge = importlib.util.module_from_spec(spec)
spec.loader.exec_module(bridge)

class NativeScope(unittest.TestCase):
    def test_required_roots_include_shared_ancestors_and_refuse_foreign_sources(self):
        from catalog import load_context
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            config = json.loads((Path(__file__).resolve().parents[2] / 'config.yaml').read_text())
            for key, name in [('connection', 'working'), ('state_connection','state')]:
                config['gateways']['local'][key]['database'] = str(root / (name+'.duckdb'))
            (root/'config.yaml').write_text(json.dumps(config))
            models = {'shared/ancestor':('arbitrary.base','SELECT 1 AS id'),
                      'a/staging':('different.staging','SELECT * FROM arbitrary.base'),
                      'a/evidence':('different.evidence','SELECT 1 AS id'),
                      'a/checks':('different.checks','SELECT * FROM arbitrary.base'),
                      'a/output':('different.output','SELECT * FROM different.staging'),
                      'b/foreign':('elsewhere.foreign','SELECT * FROM batz_input.parsed')}
            for path, (name, query) in models.items():
                target = root/'models'/(path+'.sql')
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(f'MODEL (name {name}, kind FULL); {query};')
            contract = dict(apiVersion='batz-execution/v0alpha1', pipeline='a',
                            staging='different.staging', evidence='different.evidence',
                            checks='different.checks', outputs={'events':'different.output'})
            context = load_context("native-regression", paths=[root], load_state=False)
            try:
                selected = bridge.validate_contract(context, contract)
                self.assertEqual(set(selected), {context.get_model(name).fqn for name in
                    ['arbitrary.base','different.staging','different.evidence','different.checks','different.output']})
                contract['outputs']['events'] = 'elsewhere.foreign'
                with self.assertRaisesRegex(ValueError, 'unsupported-input-scope'):
                    bridge.validate_contract(context, contract)
            finally:
                context.close()

class NativeBatch(unittest.TestCase):
    def test_recursive_selection_deduplicates_shared_test_and_detects_failures_and_omissions(self):
        from catalog import load_context
        from test_models import run_selected_tests, test_scope
        from catalog import configure_private
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root/'config.yaml').write_text((Path(__file__).resolve().parents[2]/'config.yaml').read_text())
            configure_private(root,root/'state')
            for owner,query in [('shared','SELECT 1 AS id'),('a','SELECT id FROM shared.item'),('c','SELECT id FROM shared.item'),('b','SELECT 2 AS id')]:
                model = root/'models'/owner/'item.sql'
                model.parent.mkdir(parents=True)
                model.write_text(f'MODEL (name {owner}.item, kind FULL); {query};')
                body = {'model':owner+'.item','outputs':{'query':{'rows':[{'id':1}]}}}
                if owner in ('a','c'):
                    body['inputs'] = {'shared.item':{'rows':[{'id':1}]}}
                test = root/'tests'/owner/'nested/test_item.yaml'
                test.parent.mkdir(parents=True)
                test.write_text(json.dumps({'test_'+owner:body}))
            missing = root/'tests/test_missing.yaml'
            missing.write_text(json.dumps({'test_missing':{'model':'absent.item','outputs':{'query':{'rows':[{'id':1}]}}}}))
            context = load_context("native-regression", paths=[root],load_state=False)
            try:
                self.assertIn(context.get_model('b.item').fqn, test_scope(context, {'b'}, set()))
                chosen = {context.get_model(name+'.item').fqn for name in ('a','c','shared')}
                executed = run_selected_tests(context,chosen)
                self.assertEqual(len(executed),3)
                with self.assertRaisesRegex(ValueError,'native model failure'):
                    run_selected_tests(context,None)
                with self.assertRaisesRegex(ValueError,'native model failure'):
                    run_selected_tests(context,[context.get_model('b.item').fqn])
                with self.assertRaisesRegex(ValueError,'native model failure'):
                    run_selected_tests(context,['absent.item'])
                with self.assertRaisesRegex(ValueError,'missing requested'):
                    run_selected_tests(context,['unknown.item'])
            finally:
                context.close()

class DeclarativeAdmission(unittest.TestCase):
    def test_native_seed_cannot_read_outside_admitted_content(self):
        from catalog import admitted_files
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            (root/'config.yaml').write_text((Path(__file__).resolve().parents[2]/'config.yaml').read_text())
            (root/'models/shared').mkdir(parents=True)
            (root/'outside.csv').write_text('id\n1\n')
            (root/'models/shared/seed.sql').write_text("MODEL (name common.seed, kind SEED(path '../../outside.csv'));")
            with self.assertRaisesRegex(ValueError,'seed.*admitted'):
                admitted_files(root)

    def test_executable_native_content_refused_without_touching_implementation_tools(self):
        from catalog import snapshot
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)/'source'
            root.mkdir()
            (root/'config.yaml').write_text((Path(__file__).resolve().parents[2]/'config.yaml').read_text())
            (root/'tools').mkdir()
            (root/'tools/helper.py').write_text("raise RuntimeError('must never import tooling')")
            snapshot(root,Path(scratch)/'clean')
            for relative in ('config.py','models/a/model.py','macros/bad.py','models/a/link.sql','linter/bad.py','external_models/other.yaml','schema.yaml'):
                path = root/relative
                path.parent.mkdir(parents=True,exist_ok=True)
                if path.suffix == '.sql':
                    path.symlink_to(root/'config.yaml')
                else:
                    path.write_text("raise RuntimeError('PROJECT EXECUTED')")
                with self.assertRaises(ValueError):
                    snapshot(root,Path(scratch)/'refused')
                path.unlink()

class MandatoryAudit(unittest.TestCase):
    def test_real_audit_failure_stops_build_and_keeps_prior_output(self):
        import shutil
        import duckdb
        with tempfile.TemporaryDirectory() as scratch:
            root = Path(scratch)
            source = Path(__file__).resolve().parents[2]
            parsed = root/'parsed.parquet'
            connection = duckdb.connect(config={'threads':1})
            connection.execute("COPY (SELECT 'row' AS observation_id, '{\"id\":\"invented\",\"observed_at\":\"2020-01-01T00:00:00Z\"}' AS payload) TO ? (FORMAT PARQUET)",[str(parsed)])
            connection.close()
            contract = dict(apiVersion='batz-execution/v0alpha1',pipeline='synthetic-jsonl',
                            staging='probe.records',evidence='probe.evaluation',checks='probe.required_checks',outputs={'findings':'probe.findings'})
            for attempt in (0,1):
                project = root/('project'+str(attempt))
                shutil.copytree(source/'models/synthetic-jsonl',project/'models/synthetic-jsonl')
                shutil.copyfile(source/'config.yaml',project/'config.yaml')
                output = project/'models/synthetic-jsonl/findings.sql'
                output.write_text(output.read_text().replace('kind FULL)', 'kind FULL, audits (required_event))'))
                (project/'audits').mkdir()
                (project/'audits/required.sql').write_text('AUDIT (name required_event); SELECT * FROM @this_model WHERE '+('false' if attempt==0 else 'true')+';')
                request = dict(contract=contract,artifact_root=str(root),project=str(project),
                               model_state=str(root/('state'+str(attempt))),output=str(root/('output'+str(attempt))),
                               members=[{'revision':'parsed','file':bridge.file_ref(parsed,root,1)}],
                               input_revision='parsed',input_digest='digest',attempt_id='attempt'+str(attempt),
                               evaluation_nonce='nonce'+str(attempt),binding_id='demo',revision='revision'+str(attempt))
                request_path = root/'request.json'
                request_path.write_text(json.dumps(request))
                if attempt == 0:
                    state = Path(request['model_state'])
                    state.mkdir()
                    with self.assertRaises(FileExistsError):
                        bridge.main(request_path)
                    state.rmdir()
                    bridge.main(request_path)
                    self.assertTrue((root/'output0/built.json').is_file())
                else:
                    with self.assertRaises(Exception):
                        bridge.main(request_path)
                    self.assertFalse((root/'output1/built.json').exists())
                    self.assertTrue((root/'output0/built.json').is_file())
