"""Small public setup regressions; external mise/uv processes are the boundary."""
import importlib.util
import json
import os
import re
import tomllib
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('batz_setup', Path(__file__).with_name('setup.py'))
setup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(setup)


class SetupTests(unittest.TestCase):
    def test_uv_packages_preserve_approved_versions_and_artifacts(self):
        root = Path(__file__).resolve().parents[1]
        approved = {}
        for name in ('requirements.lock', 'check-requirements.lock'):
            current = None
            for line in (root / 'runtime' / name).read_text().splitlines():
                match = re.match(r'^([A-Za-z0-9_.-]+)==([^\s\\]+)', line)
                if match:
                    current = (re.sub(r'[-_.]+', '-', match[1]).lower(), match[2])
                    approved.setdefault(current, set())
                elif '--hash=sha256:' in line:
                    self.assertIsNotNone(current)
                    approved[current].update(re.findall(r'sha256:([a-f0-9]{64})', line))
        packages = [p for p in tomllib.loads((root / 'uv.lock').read_text())['package']
                    if 'registry' in p['source']]
        self.assertEqual({(p['name'], p['version']) for p in packages}, set(approved))
        for package in packages:
            artifacts = package.get('wheels', []) + ([package['sdist']] if 'sdist' in package else [])
            hashes = {artifact['hash'].removeprefix('sha256:') for artifact in artifacts}
            self.assertTrue(hashes)
            self.assertLessEqual(hashes, approved[(package['name'], package['version'])], package['name'])

    def fixture(self, directory):
        root = Path(directory) / 'installation with spaces'
        runtime = root / 'runtime'
        runtime.mkdir(parents=True)
        source = Path(directory) / 'mise-python/bin/python3.12'
        source.parent.mkdir(parents=True)
        source.write_bytes(b'approved Python')
        go = Path(directory) / 'mise-go/bin/go'
        go.parent.mkdir(parents=True)
        go.write_bytes(b"#!/bin/sh\nprintf 'go version go1.27.3 darwin/arm64\\n'\n")
        uv = Path(directory) / 'mise-uv/uv'
        uv.parent.mkdir()
        uv.write_bytes(b'approved uv')
        lock = {'python': {'binary_sha256': {'darwin/arm64': setup.sha256(source)}},
                'uv': {'version': '0.8.12', 'binary_sha256': {'darwin/arm64': setup.sha256(uv)}},
                'mise': {'version': '2026.10.24'}, 'go': {'version': '1.27.3', 'binary_sha256': {'darwin/arm64': setup.sha256(go)}}, 'lock_files': {}}
        (runtime / 'dependencies.lock.json').write_text(json.dumps(lock))
        def output(argv, **kwargs):
            if argv[1] == 'version':
                return '2026.10.24 macos-arm64'
            return str(uv.parent if 'uv@' in argv[-1] else Path(directory)/'mise-go')
        def sync(argv, **kwargs):
            env = kwargs['env']
            self.assertNotIn('PYTHONHOME', env)
            self.assertNotIn('PYTHONPATH', env)
            self.assertNotIn('VIRTUAL_ENV', env)
            self.assertNotIn('UV_INDEX', env)
            self.assertNotIn('GOROOT', env)
            self.assertEqual(env.get('GOCACHEPROG'), '')
            self.assertEqual(env.get('GOEXPERIMENT'), '')
            self.assertEqual(env.get('GOFLAGS'), '')
            self.assertEqual(env.get('GOENV'), 'off')
            self.assertEqual(env.get('CGO_ENABLED'), '0')
            self.assertEqual(env['UV_PROJECT_ENVIRONMENT'], str((runtime / '.local/venv').resolve()))
            self.assertTrue({'--locked', '--no-build', '--no-python-downloads', '--reinstall'} <= set(argv))
            venv = runtime / '.local/venv'
            (venv / 'bin').mkdir(parents=True, exist_ok=True)
            (venv / 'pyvenv.cfg').write_text('uv = 0.8.12\ninclude-system-site-packages = false\nhome = ' + str((runtime / '.local/python/bin').resolve()) + '\n')
            (venv / 'bin/python').symlink_to(runtime / '.local/python/bin/python3.12')
        return root, source, output, sync

    def test_missing_environment_is_repaired_and_foreign_settings_cannot_redirect_sync(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source, output, sync = self.fixture(directory)
            poison = dict(PYTHONHOME='/foreign', PYTHONPATH='/foreign', VIRTUAL_ENV='/foreign',
                          UV_INDEX='https://invalid.example', UV_PROJECT_ENVIRONMENT='/foreign',
                          GOROOT='/foreign', GOCACHEPROG='/foreign/program', GOEXPERIMENT='foreign',
                          GOFLAGS='-overlay=/foreign/overlay.json', GOENV='/foreign/goenv', CGO_ENABLED='1')
            with patch.object(setup.sys, 'executable', str(source)), patch.object(setup, 'target_name', return_value='darwin/arm64'), patch.object(setup.subprocess, 'check_output', side_effect=output), patch.object(setup.subprocess, 'run', side_effect=sync), patch.dict(os.environ, poison, clear=True):
                setup.main(root)
                __import__('shutil').rmtree(root / 'runtime/.local/venv')
                setup.main(root)
            receipt = json.loads((root / 'runtime/.local/installation.json').read_text())
            self.assertEqual(receipt['schema'], 'batz-installation/v0alpha1')
            self.assertEqual(receipt['runtime_lock_sha256'], setup.sha256(root / 'runtime/dependencies.lock.json'))
            self.assertEqual(receipt['binaries']['python']['sha256'], setup.sha256(source))
            with patch.object(setup, 'target_name', return_value='darwin/arm64'):
                setup.verify_installation(root)
                config = root / 'runtime/.local/venv/pyvenv.cfg'
                original = config.read_text()
                config.write_text(original.replace('include-system-site-packages = false', 'include-system-site-packages = true'))
                with self.assertRaisesRegex(ValueError, 'private uv'):
                    setup.verify_installation(root)
                config.write_text(original)
                venv = root / 'runtime/.local/venv'
                foreign = Path(directory) / 'foreign'
                venv.rename(foreign)
                venv.symlink_to(foreign)
                with self.assertRaisesRegex(ValueError, 'foreign runtime'):
                    setup.verify_installation(root)

    def test_foreign_runtime_link_and_wrong_interpreter_refuse_before_sync(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source, output, sync = self.fixture(directory)
            local = root / 'runtime/.local'
            local.mkdir()
            (local / 'venv').symlink_to(Path(directory) / 'foreign')
            with self.assertRaisesRegex(ValueError, 'foreign runtime'):
                setup.main(root)
            (local / 'venv').unlink()
            with patch.object(setup, 'target_name', return_value='darwin/arm64'), self.assertRaisesRegex(ValueError, 'Python binary checksum'):
                setup.main(root)

    def test_changed_checkout_collector_source_refuses_the_old_installation(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source, output, sync = self.fixture(directory)
            with patch.object(setup.sys, 'executable', str(source)), patch.object(setup, 'target_name', return_value='darwin/arm64'), patch.object(setup.subprocess, 'check_output', side_effect=output), patch.object(setup.subprocess, 'run', side_effect=sync):
                setup.main(root)
                (root / '.git').write_text('fixture checkout')
                (root / 'go.mod').write_text('module invented\n')
                (root / 'go.sum').write_text('')
                collector = root / 'runtime/.local/batz-collector'
                collector.write_bytes(b'invented source collector')
                receipt_path = root / 'runtime/.local/installation.json'
                receipt = json.loads(receipt_path.read_text())
                receipt['collector_source_sha256'] = setup.collector_source_sha256(root)
                receipt['binaries']['collector'] = {'path': 'runtime/.local/batz-collector', 'sha256': setup.sha256(collector)}
                receipt_path.write_text(json.dumps(receipt))
                setup.verify_installation(root)
                (root / 'go.mod').write_text('module changed\n')
                with self.assertRaisesRegex(ValueError, 'collector source'):
                    setup.verify_installation(root)

    def test_replaced_go_compiler_is_rejected_even_when_version_is_unchanged(self):
        with tempfile.TemporaryDirectory() as directory:
            root, source, output, sync = self.fixture(directory)
            go = Path(directory) / 'mise-go/bin/go'
            with patch.object(setup.sys, 'executable', str(source)), patch.object(setup, 'target_name', return_value='darwin/arm64'), patch.object(setup.subprocess, 'check_output', side_effect=output), patch.object(setup.subprocess, 'run', side_effect=sync) as run:
                setup.main(root)
                setup.verify_installation(root)
                go.write_bytes(go.read_bytes() + b'# replaced compiler, unchanged reported version\n')
                with self.assertRaisesRegex(ValueError, 'Go binary checksum'):
                    setup.verify_installation(root)
                run.reset_mock()
                with self.assertRaisesRegex(ValueError, 'Go binary checksum'):
                    setup.main(root)
                run.assert_not_called()


if __name__ == '__main__':
    unittest.main()
