"""Selection against real Git histories and native metadata, without evaluation."""
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('selection', Path(__file__).with_name('select.py'))
selection = importlib.util.module_from_spec(spec)
spec.loader.exec_module(selection)


class AffectedJobs(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)
        self.git('init', '-b', 'main')
        self.write('config.yaml', (Path(__file__).resolve().parents[1]/'config.yaml').read_text())
        for identity in ('pipeline-a', 'pipeline-b'):
            namespace = identity.replace('-', '_')
            self.write('pipelines/' + identity + '.yaml', json.dumps({
                'apiVersion': 'batz/v0alpha2', 'kind': 'SourcePipeline', 'name': identity, 'version': 1,
                'normalization': {'project': 'repo://.',
                                  'staging': namespace + '.staging', 'evidence': namespace + '.evidence', 'checks': namespace + '.checks'},
                'routes': {'local_raw': {'input': {'engine': 'bento', 'framing': 'records-json',
                                                   'config': 'repo://collector/' + identity + '.yaml', 'params': {}},
                                         'output': {'parsed': {'format': 'parquet'}, 'normalized': {'models': {'events': namespace + '.events'}}}}},
                'explain': {'examples': [{'id': 'example', 'route': 'local_raw', 'input': 'repo://fixtures/' + identity + '.json',
                                          'expectations': 'repo://fixtures/' + identity + '.expected.json', 'sha256': 'a'*64}]}
            }))
            for model in ('staging','evidence','checks','events'):
                self.write('models/' + identity + '/' + model + '.sql', f'MODEL (name {namespace}.{model}, kind FULL); SELECT 1 AS id;')
            self.write('collector/' + identity + '.yaml', '{}\n')
            self.write('fixtures/' + identity + '.json', '{"Records":[]}\n')
            self.write('fixtures/' + identity + '.expected.json', '{}\n')
        self.base = self.commit()

    def git(self, *args):
        return subprocess.check_output(['git', '-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid', *args], cwd=self.root, stderr=subprocess.DEVNULL).decode().strip()

    def write(self, path, content):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def commit(self):
        self.git('add', '.')
        self.git('commit', '-qm', 'invented selection fixture')
        return self.git('rev-parse', 'HEAD')

    def selected(self):
        return selection.select_changes(self.root, self.base, self.commit())

    def test_pipeline_only_change_selects_no_other_pipeline(self):
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT 2 AS id;')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertNotIn('e2e', jobs)
        self.assertTrue(jobs['fast'])
        self.assertFalse(jobs['app'])

    def test_unconsumed_shared_model_broadens_native_tests(self):
        self.write('models/shared/orphan.sql', 'MODEL (name common.orphan, kind FULL); SELECT 2 AS id;')
        self.write('tests/shared/test_orphan.yaml', json.dumps({'orphan': {'model':'common.orphan','outputs':{'query':{'rows':[{'id':1}]}}}}))
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a','pipeline-b'])
        self.assertTrue(any('unconsumed native' in reason for reason in jobs['reasons']))

    def test_owned_helper_outside_execution_scope_selects_owner(self):
        self.write('models/pipeline-a/helper.sql', 'MODEL (name pipeline_a.helper, kind FULL); SELECT 1 AS id;')
        self.assertEqual(self.selected()['models'], ['pipeline-a'])

    def test_native_test_only_skips_adapters_and_e2e(self):
        self.write('tests/pipeline-a/test_events.yaml', json.dumps({'events': {'model':'pipeline_a.events','outputs':{'query':{'rows':[{'id':1}]}}}}))
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertEqual(jobs['adapters'], [])
        self.assertFalse(jobs['fast'])
        self.assertNotIn('e2e', jobs)

    def test_shared_app_selects_critical_runtime_without_catalog_e2e(self):
        self.write('internal/app/app.go', 'package app\\n')
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], [])
        self.assertNotIn('e2e', jobs)

    def test_docs_only_changes_start_no_native_lane(self):
        self.write('docs/guide.md', 'Guide')
        jobs = self.selected()
        self.assertTrue(jobs['docs'])
        self.assertFalse(jobs['fast'])
        self.assertFalse(jobs['app'])
        self.assertEqual(jobs['models'], [])
        self.assertNotIn('e2e', jobs)

    def test_unknown_or_selector_changes_broaden_contracts_not_catalog_e2e(self):
        self.write('ci/select.py', '# changed selection')
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], ['pipeline-a', 'pipeline-b'])
        self.assertNotIn('e2e', jobs)

    def test_rename_keeps_native_identity_and_owner(self):
        self.git('mv', 'models/pipeline-a/events.sql', 'models/pipeline-a/renamed.sql')
        self.assertEqual(self.selected()['models'], ['pipeline-a'])

    def test_deleted_required_root_fails_head_discovery(self):
        (self.root / 'models/pipeline-a/events.sql').unlink()
        with self.assertRaises(Exception):
            self.selected()

    def test_removed_shared_edge_checks_former_consumer(self):
        self.write('models/shared/s.sql', 'MODEL (name common.s, kind FULL); SELECT 1 AS id;')
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT * FROM common.s;')
        self.base = self.commit()
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT 2 AS id;')
        (self.root/'models/shared/s.sql').unlink()
        self.assertEqual(self.selected()['models'], ['pipeline-a'])

    def test_removed_pipeline_broadens_remaining_contracts(self):
        (self.root / 'pipelines/pipeline-a.yaml').unlink()
        jobs = self.selected()
        self.assertEqual(jobs['removed_pipelines'], ['pipeline-a'])
        self.assertEqual(jobs['models'], ['pipeline-b'])
        self.assertTrue(jobs['app'])

    def test_shared_dependency_selects_consumers_only(self):
        self.write('models/shared/s.sql', 'MODEL (name common.s, kind FULL); SELECT 1 AS id;')
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT * FROM common.s;')
        self.base = self.commit()
        self.write('models/shared/s.sql', 'MODEL (name common.s, kind FULL); SELECT 2 AS id;')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertFalse(jobs['app'])

    def test_independent_pipeline_addition_needs_no_registry(self):
        value = json.loads((self.root/'pipelines/pipeline-a.yaml').read_text())
        value['name'] = 'new-source'
        for name in ('staging','evidence','checks'):
            value['normalization'][name] = 'unrelated_schema.'+name
        value['routes']['local_raw']['output']['normalized']['models'] = {'events':'unrelated_schema.events'}
        self.write('pipelines/new-source/pipeline.yaml',json.dumps(value))
        for name in ('staging','evidence','checks','events'):
            self.write('models/new-source/'+name+'.sql',f'MODEL (name unrelated_schema.{name}, kind FULL); SELECT 1 AS id;')
        self.write('tests/new-source/test_events.yaml',json.dumps({'new_test':{'model':'unrelated_schema.events','outputs':{'query':{'rows':[{'id':1}]}}}}))
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertEqual(jobs['models'],['new-source'])
        self.assertEqual(jobs['adapters'],['new-source'])
        self.assertNotIn('e2e', jobs)
        self.assertFalse(jobs['app'])

    def test_old_layout_base_broadens_with_reason(self):
        (self.root/'config.yaml').unlink()
        self.base = self.commit()
        self.write('config.yaml', (Path(__file__).resolve().parents[1]/'config.yaml').read_text())
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a','pipeline-b'])
        self.assertTrue(any('unsupported-base' in reason for reason in jobs['reasons']))

    def test_merge_base_excludes_changes_unique_to_the_target_branch(self):
        self.write('models/pipeline-b/events.sql', 'SELECT 2;')
        target = self.commit()
        self.git('checkout', '-qb', 'feature', self.base)
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT 3 AS id;')
        head = self.commit()
        jobs = selection.select_changes(self.root, target, head)
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertNotIn('e2e', jobs)

    def test_declared_source_reference_is_not_docs_by_extension(self):
        path = self.root / 'pipelines/pipeline-a.yaml'
        value = json.loads(path.read_text())
        value['explain']['examples'][0]['input'] = 'repo://fixtures/source.md'
        path.write_text(json.dumps(value))
        self.write('fixtures/source.md', '{"Records":[]}')
        self.base = self.commit()
        self.write('fixtures/source.md', '{"Records":[{}]}')
        self.assertEqual(self.selected()['models'], ['pipeline-a'])

    def test_testdata_neighbour_selects_its_pipeline_and_shared_app(self):
        value = json.loads((self.root / 'pipelines/pipeline-a.yaml').read_text())
        value['name'] = 'probe'
        for model in ('staging','evidence','checks','events'):
            self.write('models/probe/'+model+'.sql', f'MODEL (name probe.{model}, kind FULL); SELECT 1 AS id;')
        for key in ('staging','evidence','checks'):
            value['normalization'][key] = 'probe.'+key
        value['routes']['local_raw']['output']['normalized']['models']['events'] = 'probe.events'
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        self.base = self.commit()
        self.write('internal/app/testdata/probe/undeclared.jsonl', '{}\n')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['probe'])
        self.assertNotIn('e2e', jobs)
        self.assertTrue(jobs['app'])
        self.base = self.git('rev-parse', 'HEAD')
        self.write('internal/app/testdata/probe/README.md', 'Fixture notes only')
        jobs = self.selected()
        self.assertTrue(jobs['docs'])
        self.assertFalse(jobs['app'])
        self.assertEqual(jobs['models'], [])
        self.assertNotIn('e2e', jobs)

    def test_bridge_selects_native_contracts_and_shared_fixture_coverage(self):
        value = json.loads((self.root / 'pipelines/pipeline-a.yaml').read_text())
        value['name'] = 'probe'
        for model in ('staging','evidence','checks','events'):
            self.write('models/probe/'+model+'.sql', f'MODEL (name probe.{model}, kind FULL); SELECT 1 AS id;')
        for key in ('staging','evidence','checks'):
            value['normalization'][key] = 'probe.'+key
        value['routes']['local_raw']['output']['normalized']['models']['events'] = 'probe.events'
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        self.base = self.commit()
        self.write('internal/app/explain.go', '// shared preview orchestration')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a', 'pipeline-b', 'probe'])
        self.assertNotIn('e2e', jobs)

    def test_shared_native_helper_selects_shared_coverage(self):
        self.write('internal/app/native_test.go', '// shared reader and fixture helpers')
        jobs = self.selected()
        self.assertTrue(jobs['app'])
        self.assertNotIn('e2e', jobs)

    def test_fixture_model_and_native_test_select_shared_not_probe_e2e(self):
        value = json.loads((self.root / 'pipelines/pipeline-a.yaml').read_text())
        value['name'] = 'probe'
        for model in ('staging', 'evidence', 'checks', 'events'):
            self.write('models/probe/'+model+'.sql', f'MODEL (name probe.{model}, kind FULL); SELECT 1 AS id;')
        for key in ('staging', 'evidence', 'checks'):
            value['normalization'][key] = 'probe.'+key
        value['routes']['local_raw']['output']['normalized']['models']['events'] = 'probe.events'
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        self.base = self.commit()
        self.write('models/probe/events.sql', 'MODEL (name probe.events, kind FULL); SELECT 2 AS id;')
        jobs = self.selected()
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['adapters'], ['probe'])
        self.assertNotIn('e2e', jobs)
        self.base = self.git('rev-parse', 'HEAD')
        self.write('tests/probe/test_events.yaml', json.dumps({'fixture': {'model': 'probe.events', 'outputs': {'query': {'rows': [{'id': 2}]}}}}))
        jobs = self.selected()
        self.assertFalse(jobs['app'])
        self.assertEqual(jobs['adapters'], [])
        self.assertFalse(jobs['fast'])
        self.assertEqual(jobs['models'], ['probe'])
        self.assertNotIn('e2e', jobs)

    def test_core_retry_and_moved_replay_select_shared_coverage(self):
        for path in ('internal/run/recovery.go', 'internal/app/app_replay_test.go'):
            self.base = self.git('rev-parse', 'HEAD')
            self.write(path, '// shared behavior')
            jobs = self.selected()
            self.assertTrue(jobs['app'])
            self.assertNotIn('e2e', jobs)

    def test_former_vendor_test_path_is_now_shared_code(self):
        self.write('internal/app/pipeline_a_e2e_test.go', '// source wiring')
        jobs = self.selected()
        self.assertTrue(jobs['app'])
        self.assertNotIn('e2e', jobs)


    def test_shared_sql_consumed_by_synthetic_journey_selects_app_on_removed_edge(self):
        value = json.loads((self.root/'pipelines/pipeline-a.yaml').read_text())
        self.git('mv', 'pipelines/pipeline-a.yaml', 'internal-probe.yaml')
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        (self.root/'internal-probe.yaml').unlink()
        self.write('models/shared/s.sql', 'MODEL (name common.s, kind FULL); SELECT 1 AS id;')
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT * FROM common.s;')
        self.base = self.commit()
        self.write('models/pipeline-a/events.sql', 'MODEL (name pipeline_a.events, kind FULL); SELECT 2 AS id;')
        (self.root/'models/shared/s.sql').unlink()
        jobs = self.selected()
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertTrue(any('shared runtime model impact' in r for r in jobs['reasons']))

    def test_native_test_and_source_edit_still_select_fast(self):
        self.write('tests/pipeline-a/test_events.yaml', json.dumps({'events': {'model':'pipeline_a.events','outputs':{'query':{'rows':[{'id':1}]}}}}))
        self.write('collector/pipeline-a.yaml', '{"updated":true}')
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertEqual(jobs['adapters'], ['pipeline-a'])

    def test_core_deletion_selects_shared_app(self):
        self.write('internal/run/helper.go', '// original')
        self.base = self.commit()
        (self.root/'internal/run/helper.go').unlink()
        self.assertTrue(self.selected()['app'])

    def test_global_config_selects_shared_app_and_all_contracts(self):
        self.write('config.yaml', (self.root/'config.yaml').read_text()+'\n')
        jobs = self.selected()
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], ['pipeline-a', 'pipeline-b'])
        self.assertEqual(jobs['adapters'], ['pipeline-a', 'pipeline-b'])
        self.assertNotIn('e2e', jobs)

if __name__ == '__main__':
    unittest.main()
