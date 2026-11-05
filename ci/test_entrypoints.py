"""Public wrapper refusal checks; requested work must never silently pass."""
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[1]


class RequestedLanes(unittest.TestCase):
    def test_unknown_requested_pipeline_is_refused(self):
        result = subprocess.run([str(ROOT / 'ci/test'), 'models', 'missing-pipeline'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn('missing-pipeline', result.stderr)

    def test_per_pipeline_e2e_lane_is_retired_visibly(self):
        result = subprocess.run([str(ROOT / 'ci/test'), 'e2e', 'aws-cloudtrail'], capture_output=True, text=True)
        self.assertEqual(result.returncode, 2, result.stdout + result.stderr)
        self.assertIn('invalid choice', result.stderr)


class SharedExecution(unittest.TestCase):
    """The public lane must reject a successful tool exit with missing/skipped work."""
    def run_events(self, events, required=()):
        import importlib.util
        import json
        import os
        import tempfile
        from unittest.mock import patch
        spec = importlib.util.spec_from_file_location('lanes', ROOT / 'ci/test.py')
        lanes = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(lanes)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / 'internal/app/app_runtime_test.go'
            source.parent.mkdir(parents=True)
            source.write_text('//go:build batz_app\n')
            tool = root / 'go'
            output = '\n'.join(json.dumps(event) for event in events) + '\n'
            tool.write_text('#!/bin/sh\ncat <<\'EVENTS\'\n' + output + 'EVENTS\n')
            tool.chmod(0o700)
            with patch.object(lanes, 'ROOT', root), patch.dict(os.environ, PATH=str(root)+os.pathsep+os.environ['PATH']):
                lanes.go_tests(['go', 'test', '-json'], 'TestApp', required=required)

    def test_required_shared_journey_cannot_disappear_behind_other_passing_tests(self):
        with self.assertRaisesRegex(ValueError, 'required'):
            self.run_events([{'Action':'run','Test':'TestAppHelper'},
                             {'Action':'pass','Test':'TestAppHelper'}, {'Action':'pass'}],
                            required={'TestAppSharedPipelineJourney'})

    def test_one_empty_package_cannot_hide_behind_another_passing_package(self):
        with self.assertRaisesRegex(ValueError, 'requested package'):
            self.run_events([{'Action':'start','Package':'a'}, {'Action':'start','Package':'b'},
                             {'Action':'run','Package':'a','Test':'TestAppExample'},
                             {'Action':'pass','Package':'a','Test':'TestAppExample'},
                             {'Action':'pass','Package':'a'}, {'Action':'pass','Package':'b'}])

    def test_no_matching_test_is_refused(self):
        with self.assertRaisesRegex(ValueError, 'executed.*TestApp'):
            self.run_events([{'Action': 'pass', 'Package': 'example/internal/app'}])

    def test_selected_skip_including_subtest_is_refused(self):
        for name in ('TestAppExample', 'TestAppExample/unsupported'):
            with self.subTest(name=name), self.assertRaisesRegex(ValueError, 'skip'):
                self.run_events([{'Action': 'skip', 'Test': name},
                                 {'Action': 'pass', 'Test': 'TestAppExample'}, {'Action': 'pass'}])

    def test_missing_package_result_is_refused(self):
        with self.assertRaisesRegex(ValueError, 'package'):
            self.run_events([{'Action': 'run', 'Test': 'TestAppExample'},
                             {'Action': 'pass', 'Test': 'TestAppExample'}])

    def test_executed_matching_pass_and_package_pass_are_accepted(self):
        self.run_events([{'Action': 'run', 'Test': 'TestAppExample'},
                         {'Action': 'output', 'Output': 'readable assertion output\n'},
                         {'Action': 'pass', 'Test': 'TestAppExample'}, {'Action': 'pass'}])


class ManifestContracts(unittest.TestCase):
    def test_selected_manifest_uses_production_schema_and_supported_version(self):
        import copy
        from pipelines import validate_manifest
        from ruamel.yaml import YAML
        value = YAML(typ='safe').load((ROOT/'pipelines/aws-cloudtrail/pipeline.yaml').read_text())
        validate_manifest(value)
        mutations = [lambda m: m['explain']['examples'][0].pop('expectations'),
                     lambda m: m['routes']['local_raw']['input']['params']['path'].update(type='unknown'),
                     lambda m: m.update(version=2),
                     lambda m: m['normalization'].update(project='repo://other')]
        for mutate in mutations:
            candidate = copy.deepcopy(value)
            mutate(candidate)
            with self.subTest(manifest=candidate), self.assertRaises(ValueError):
                validate_manifest(candidate)


if __name__ == '__main__':
    unittest.main()
