"""Artifact admission at the CI provisioning boundary (no network)."""
import hashlib
import importlib.util
from pathlib import Path
import tempfile
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


if __name__ == '__main__':
    unittest.main()
