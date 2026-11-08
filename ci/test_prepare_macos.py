"""Artifact admission at the CI provisioning boundary (no network)."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
import json
import os
from unittest.mock import patch
import unittest


class ProvisioningTests(unittest.TestCase):
    def test_corrupt_cached_download_is_rejected_before_use(self):
        spec = importlib.util.spec_from_file_location('prepare_macos', Path(__file__).with_name('prepare-macos.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            cached = Path(directory) / 'archive.tar.gz'
            cached.write_bytes(b'corrupt cached download')
            with self.assertRaisesRegex(ValueError, 'checksum mismatch'):
                module.verified_download('https://invalid.example/unused', cached,
                                         hashlib.sha256(b'expected bytes').hexdigest())
            self.assertEqual(cached.read_bytes(), b'corrupt cached download')

    def test_valid_cached_download_needs_no_network(self):
        spec = importlib.util.spec_from_file_location('prepare_macos', Path(__file__).with_name('prepare-macos.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            cached = Path(directory) / 'archive.tar.gz'
            cached.write_bytes(b'reviewed artifact')
            module.verified_download('https://invalid.example/unused', cached,
                                     hashlib.sha256(cached.read_bytes()).hexdigest())

    def test_preparation_uses_locked_uv_and_ignores_foreign_python_environment(self):
        spec = importlib.util.spec_from_file_location('prepare_macos', Path(__file__).with_name('prepare-macos.py'))
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            local = root / 'tools'
            runtime = root / 'runtime'
            runtime.mkdir()
            lock = {'mise': {'version': 'test', 'artifacts': {'darwin/arm64': {'url': 'https://invalid/mise', 'sha256': hashlib.sha256(b'mise').hexdigest()}}}}
            cache = root / 'cache'
            cache.mkdir()
            (cache / 'mise-test').write_bytes(b'mise')
            (runtime / 'dependencies.lock.json').write_text(json.dumps(lock))
            calls = []
            def run(argv, **kwargs):
                calls.append((argv, kwargs))
                return __import__('subprocess').CompletedProcess(argv, 0)
            env = dict(BATZ_TOOLCHAIN_ROOT=str(local), BATZ_DOWNLOAD_CACHE=str(root/'cache'),
                       PYTHONHOME='/foreign', PYTHONPATH='/foreign', VIRTUAL_ENV='/foreign', UV_PROJECT_ENVIRONMENT='/foreign')
            with patch.object(module, 'ROOT', root), patch.object(module.platform, 'system', return_value='Darwin'), patch.object(module.platform, 'machine', return_value='arm64'), patch.object(module.subprocess, 'run', side_effect=run), patch.dict(os.environ, env):
                module.main()
            for argv, kwargs in calls:
                for variable in ('PYTHONHOME', 'PYTHONPATH', 'VIRTUAL_ENV', 'UV_PROJECT_ENVIRONMENT'):
                    self.assertNotIn(variable, kwargs['env'])
            self.assertTrue(any('mise' in str(argv[0]) and 'install' in argv for argv, _ in calls), 'preparation must delegate to normal mise install')


if __name__ == '__main__':
    unittest.main()
