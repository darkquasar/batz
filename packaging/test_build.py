"""Bounded public producer contracts; native installed execution joins setup at J."""
import pathlib
import hashlib
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
BUILD = ROOT / "packaging/build.py"
spec = importlib.util.spec_from_file_location("batz_package_build", BUILD)
builder = importlib.util.module_from_spec(spec)
spec.loader.exec_module(builder)


def protocol_archive(directory, target):
    """Invented candidate protocol bytes; these do not qualify native execution."""
    name = 'batz-v0.0.0-test.1-' + target.replace('/', '-')
    stage = directory / name
    payload = {
        'bin/batz': b'invented CLI', 'runtime/.local/batz-collector': b'invented collector',
        'mise.toml': b'fixture', 'mise.lock': b'fixture', 'pyproject.toml': b'fixture',
        'uv.lock': b'fixture', 'ci/setup.py': b'fixture', 'config.yaml': b'fixture',
        'go.mod': b'module fixture', 'go.sum': b'fixture',
        'runtime/dependencies.lock.json': b'{}', 'runtime/requirements.lock': b'fixture',
        'runtime/check-requirements.lock': b'fixture', 'runtime/sqlmesh/runner.py': b'fixture',
        'runtime/sqlmesh/catalog.py': b'fixture', 'runtime/sqlmesh/inspect_preview.py': b'fixture',
        'docs/usage/install.md': b'fixture', 'docs/usage/ci.md': b'fixture',
        'models/probe.sql': b'SELECT 1', 'pipelines/probe/pipeline.yaml': b'fixture',
        'schemas/probe.json': b'{}',
    }
    for relative, data in payload.items():
        path = stage / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
    inventory = {'schema': 'batz-release-content/v0alpha1', 'version': 'v0.0.0-test.1',
                 'source_sha': '1' * 40, 'target': target, 'development': True, 'dirty': True,
                 'source_tree_sha256': '2' * 64,
                 'files': {name: hashlib.sha256(data).hexdigest() for name, data in payload.items()}}
    (stage / 'release-content.json').write_text(json.dumps(inventory))
    archive = directory / (name + '.tar.gz')
    builder.write_tar(stage, archive, name)
    return archive, stage


class PackageTests(unittest.TestCase):
    def test_archive_refuses_ignored_native_input_outside_source_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = pathlib.Path(temporary)
            _, source = protocol_archive(directory, 'darwin/arm64')
            source = source.resolve()
            (source / '.gitignore').write_text('pipelines/private.json\n')
            for arguments in (['init', '-q'], ['add', '.'],
                              ['-c', 'user.name=Fixture', '-c', 'user.email=fixture@example.invalid',
                               'commit', '-qm', 'invented source']):
                subprocess.run(['git', *arguments], cwd=source, check=True, capture_output=True)
            (source / 'pipelines/private.json').write_text('{"private": "local-only"}')
            with self.assertRaisesRegex(ValueError, 'outside admitted source'):
                builder.content_paths(source)

    def test_archive_refuses_untrusted_compiler_before_executing_it(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = pathlib.Path(temporary)
            marker = directory / 'executed'
            compiler = directory / 'go'
            compiler.write_text('#!/bin/sh\nprintf invoked > "' + str(marker) + '"\n'
                                'if [ "$1" = env ]; then printf "go1.27.3\\n"; exit 0; fi\nexit 42\n')
            compiler.chmod(0o700)
            source_sha = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
            result = subprocess.run([sys.executable, str(BUILD), 'archive', '--root', str(ROOT),
                                     '--output', str(directory), '--version', 'v0.0.0-test.1',
                                     '--source-sha', source_sha, '--target', 'darwin/arm64',
                                     '--go', str(compiler), '--development'], text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertFalse(marker.exists(), 'untrusted compiler executed before integrity admission')
            self.assertIn('compiler integrity mismatch', result.stderr)

    def test_missing_setup_metadata_refuses_incomplete_archive(self):
        with tempfile.TemporaryDirectory() as directory:
            result = subprocess.run([
                sys.executable, str(BUILD), "archive", "--root", directory,
                "--output", directory, "--version", "v0.0.0-test.1",
                "--source-sha", "0" * 40, "--target", "darwin/arm64", "--go", "/not/used",
                "--development",
            ], text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("missing package input: mise.toml", result.stderr)
            self.assertEqual(list(pathlib.Path(directory).glob("*.tar.gz")), [])

    def test_candidate_binds_both_archive_bytes_and_preserves_development_identity(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = pathlib.Path(temporary)
            darwin, _ = protocol_archive(directory, 'darwin/arm64')
            linux, _ = protocol_archive(directory, 'linux/amd64')
            output = directory / 'candidate.json'
            result = subprocess.run([sys.executable, str(BUILD), 'candidate', '--archive', str(darwin),
                                     '--archive', str(linux), '--docker-digest', 'sha256:' + '3' * 64,
                                     '--output', str(output)], text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            manifest = json.loads(output.read_bytes())
            self.assertEqual(manifest['schema'], 'batz-release-candidate/v0alpha1')
            self.assertTrue(manifest['development'])
            self.assertTrue(manifest['dirty'])
            self.assertEqual({row['target'] for row in manifest['targets']}, {'darwin/arm64', 'linux/amd64'})
            self.assertEqual(manifest['targets'][0]['sha256'], hashlib.sha256(darwin.read_bytes()).hexdigest())
            self.assertNotIn('readiness', manifest)

    def test_candidate_refuses_tampered_native_content(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = pathlib.Path(temporary)
            archive, stage = protocol_archive(directory, 'darwin/arm64')
            (stage / 'models/probe.sql').write_text('SELECT 2')
            builder.write_tar(stage, archive, stage.name)
            with self.assertRaisesRegex(ValueError, 'digest mismatch'):
                builder.inspect_archive(archive)


if __name__ == "__main__":
    unittest.main()
