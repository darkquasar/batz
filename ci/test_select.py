"""Selection behavior against small real Git histories; starts no native engine."""
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
        for identity in ('pipeline-a', 'pipeline-b'):
            namespace = identity.replace('-', '_')
            self.write('pipelines/' + identity + '.yaml', json.dumps({
                'apiVersion': 'batz/v0alpha2', 'kind': 'SourcePipeline', 'name': identity, 'version': 1,
                'normalization': {'project': 'repo://normalization/' + identity,
                                  'staging': namespace + '.staging', 'evidence': namespace + '.evidence', 'checks': namespace + '.checks'},
                'routes': {'local_raw': {'input': {'engine': 'bento', 'framing': 'records-json',
                                                   'config': 'repo://collector/' + identity + '.yaml', 'params': {}},
                                         'output': {'parsed': {'format': 'parquet'}, 'normalized': {'models': {'events': namespace + '.events'}}}}},
                'explain': {'examples': [{'id': 'example', 'route': 'local_raw', 'input': 'repo://fixtures/' + identity + '.json',
                                          'expectations': 'repo://fixtures/' + identity + '.expected.json', 'sha256': 'a'*64}]}
            }))
            self.write('normalization/' + identity + '/models/events.sql', 'SELECT 1;\n')
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
        self.write('normalization/pipeline-a/models/events.sql', 'SELECT 2;\n')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertEqual(jobs['e2e'], ['pipeline-a'])
        self.assertTrue(jobs['fast'])
        self.assertFalse(jobs['app'])

    def test_shared_app_selects_critical_runtime_without_catalog_e2e(self):
        self.write('internal/app/app.go', 'package app\\n')
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], [])
        self.assertEqual(jobs['e2e'], [])

    def test_docs_only_changes_start_no_native_lane(self):
        self.write('docs/guide.md', 'Guide')
        jobs = self.selected()
        self.assertTrue(jobs['docs'])
        self.assertFalse(jobs['fast'])
        self.assertFalse(jobs['app'])
        self.assertEqual(jobs['models'], [])
        self.assertEqual(jobs['e2e'], [])

    def test_unknown_or_selector_changes_broaden_contracts_not_catalog_e2e(self):
        self.write('ci/select.py', '# changed selection')
        jobs = self.selected()
        self.assertTrue(jobs['fast'])
        self.assertTrue(jobs['app'])
        self.assertEqual(jobs['models'], ['pipeline-a', 'pipeline-b'])
        self.assertEqual(jobs['e2e'], ['pipeline-a'])

    def test_renamed_project_uses_both_snapshots_ownership(self):
        self.git('mv', 'normalization/pipeline-a', 'normalization/renamed-a')
        path = self.root / 'pipelines/pipeline-a.yaml'
        value = json.loads(path.read_text())
        value['normalization']['project'] = 'repo://normalization/renamed-a'
        path.write_text(json.dumps(value))
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertEqual(jobs['e2e'], ['pipeline-a'])

    def test_deleted_referenced_model_still_selects_its_pipeline(self):
        (self.root / 'normalization/pipeline-a/models/events.sql').unlink()
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertEqual(jobs['e2e'], ['pipeline-a'])

    def test_removed_pipeline_broadens_remaining_contracts(self):
        (self.root / 'pipelines/pipeline-a.yaml').unlink()
        jobs = self.selected()
        self.assertEqual(jobs['removed_pipelines'], ['pipeline-a'])
        self.assertEqual(jobs['models'], ['pipeline-b'])
        self.assertTrue(jobs['app'])

    def test_shared_project_references_select_all_owners(self):
        path = self.root / 'pipelines/pipeline-b.yaml'
        value = json.loads(path.read_text())
        value['normalization']['project'] = 'repo://normalization/pipeline-a'
        path.write_text(json.dumps(value))
        self.base = self.commit()
        self.write('normalization/pipeline-a/models/events.sql', 'SELECT 2;')
        self.assertEqual(self.selected()['models'], ['pipeline-a', 'pipeline-b'])

    def test_merge_base_excludes_changes_unique_to_the_target_branch(self):
        self.write('normalization/pipeline-b/models/events.sql', 'SELECT 2;')
        target = self.commit()
        self.git('checkout', '-qb', 'feature', self.base)
        self.write('normalization/pipeline-a/models/events.sql', 'SELECT 3;')
        head = self.commit()
        jobs = selection.select_changes(self.root, target, head)
        self.assertEqual(jobs['models'], ['pipeline-a'])
        self.assertEqual(jobs['e2e'], ['pipeline-a'])

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
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        self.base = self.commit()
        self.write('internal/app/testdata/probe/undeclared.jsonl', '{}\n')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['probe'])
        self.assertEqual(jobs['e2e'], ['probe'])
        self.assertTrue(jobs['app'])

    def test_bridge_uses_fixed_production_and_testdata_probes(self):
        value = json.loads((self.root / 'pipelines/pipeline-a.yaml').read_text())
        value['name'] = 'probe'
        self.write('internal/app/testdata/probe/pipeline.yaml', json.dumps(value))
        self.base = self.commit()
        self.write('internal/app/explain.go', '// shared preview orchestration')
        jobs = self.selected()
        self.assertEqual(jobs['models'], ['pipeline-a', 'pipeline-b', 'probe'])
        self.assertEqual(jobs['e2e'], ['pipeline-a', 'probe'])


if __name__ == '__main__':
    unittest.main()
