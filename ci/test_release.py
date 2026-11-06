"""Release trust boundaries: real local Git/files, fake external services only."""
import copy
import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('batz_release', Path(__file__).with_name('release.py'))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
OWNER = {'login': 'darkquasar', 'id': 28435534}


class GitHub:
    def user(self, login):
        return OWNER if login == OWNER['login'] else {'login': login, 'id': 9}


class GitFixture(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='batz-release-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.git('init', '-q', '-b', 'main')
        self.git('config', 'user.email', 'test@example.invalid')
        self.git('config', 'user.name', 'Invented')
        (self.root / 'input').write_text('one')
        self.git('add', 'input'); self.git('commit', '-qm', 'one')
        self.sha = self.git('rev-parse', 'HEAD')
        self.git('remote', 'add', 'origin', str(self.root))
        self.git('tag', 'v1.2.3')
        self.env = {'GITHUB_REPOSITORY': 'darkquasar/batz', 'GITHUB_EVENT_NAME': 'push',
                    'GITHUB_REF': 'refs/tags/v1.2.3', 'GITHUB_SHA': self.sha,
                    'GITHUB_ACTOR': 'darkquasar', 'GITHUB_ACTOR_ID': '28435534',
                    'GITHUB_TRIGGERING_ACTOR': 'darkquasar'}
        self.event = self.root / 'event.json'
        self.event.write_text(json.dumps({'sender': OWNER, 'repository': {'full_name': 'darkquasar/batz'},
                                        'ref': self.env['GITHUB_REF'], 'deleted': False, 'forced': False}))
        self.env['GITHUB_EVENT_PATH'] = str(self.event)
        self.github = GitHub()

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, text=True, stderr=subprocess.DEVNULL).strip()


class Authorization(GitFixture):
    def test_authorize_cli_authenticates_read_only_lookup_and_still_checks_owner(self):
        test = self

        class Response:
            def __init__(self, user): self.user = user
            def __enter__(self): return self
            def __exit__(self, *args): pass
            def read(self): return json.dumps(self.user).encode()

        class Lookup:
            def __init__(self, user): self.user = user
            def open(self, request, timeout):
                test.assertEqual(request.full_url, 'https://api.github.com/users/darkquasar')
                test.assertEqual(request.get_method(), 'GET')
                test.assertEqual(request.get_header('Authorization'), 'Bearer invented-read-token')
                return Response(self.user)

        for user in (OWNER, dict(OWNER, id=9)):
            with self.subTest(user=user), \
                    patch.dict(os.environ, dict(self.env, GH_TOKEN='invented-read-token'), clear=True), \
                    patch.object(release, '__file__', str(self.root/'ci/release.py')), \
                    patch.object(release.urllib.request, 'build_opener', return_value=Lookup(user)), \
                    contextlib.redirect_stdout(io.StringIO()) as output:
                if user == OWNER:
                    self.assertEqual(release.main(['authorize']), 0)
                    self.assertEqual(json.loads(output.getvalue())['source_sha'], self.sha)
                else:
                    with self.assertRaisesRegex(ValueError, 'owner'):
                        release.main(['authorize'])

    def test_only_owner_can_authorize_a_tag(self):
        self.env['GITHUB_ACTOR_ID'] = '9'
        with self.assertRaisesRegex(ValueError, 'owner'):
            release.authorize(self.root, self.env, self.github)

    def test_stable_and_annotated_prerelease_resolve_exact_commit(self):
        self.assertEqual(release.authorize(self.root, self.env, self.github)['source_sha'], self.sha)
        self.git('tag', '-a', 'v2.0.0-rc.1', '-m', 'candidate')
        self.env['GITHUB_REF'] = 'refs/tags/v2.0.0-rc.1'
        event = json.loads(self.event.read_text()); event['ref'] = self.env['GITHUB_REF']
        self.event.write_text(json.dumps(event))
        self.assertEqual(release.authorize(self.root, self.env, self.github)['version'], 'v2.0.0-rc.1')

    def test_non_tip_main_ancestor_can_enter_qualification_without_readiness(self):
        (self.root/'input').write_text('new main')
        self.git('commit', '-qam', 'advance main')
        self.git('checkout', '--detach', self.sha)
        self.assertEqual(release.authorize(self.root, self.env, self.github)['source_sha'], self.sha)

    def test_refuses_malformed_foreign_manual_and_changed_refs(self):
        for key, value in [('GITHUB_REF', 'refs/tags/v01.2.3'), ('GITHUB_REF', 'refs/tags/v1.2.3-01'),
                           ('GITHUB_REF', 'refs/tags/v1.2.3+build'), ('GITHUB_REF', 'refs/heads/main'),
                           ('GITHUB_REPOSITORY', 'other/batz'), ('GITHUB_TRIGGERING_ACTOR', 'other'),
                           ('GITHUB_EVENT_NAME', 'workflow_dispatch'), ('GITHUB_SHA', 'f'*40)]:
            with self.subTest(key=key, value=value), self.assertRaises(ValueError):
                release.authorize(self.root, dict(self.env, **{key: value}), self.github)
        self.git('checkout', '-qb', 'unmerged')
        (self.root/'input').write_text('other'); self.git('commit', '-qam', 'other')
        other = self.git('rev-parse', 'HEAD'); self.git('tag', '-f', 'v1.2.3')
        with self.assertRaisesRegex(ValueError, 'outside'):
            release.authorize(self.root, dict(self.env, GITHUB_SHA=other), self.github)

class ProofGitHub(GitHub):
    def __init__(self, sha='a'*40):
        self.sha = sha
        self.record = {'schema': 'batz-release-readiness/v0alpha1', 'source_sha': sha,
                       'candidate_sha256': 'b'*64, 'source_tree_sha256': 'c'*64,
                       'content_sha256': {'darwin/arm64': 'd'*64, 'linux/amd64': 'e'*64},
                       'docker_digest': 'sha256:'+'f'*64,
                       'review': {'disposition': 'accepted', 'report_sha256': '1'*64},
                       'checks': [{'lane': lane, 'run_id': index, 'check_id': index, 'run_attempt': 1,
                                   'source_tree_sha256': 'c'*64, 'candidate_sha256': 'b'*64}
                                  for index, lane in enumerate(sorted(release.LANES), 1)]}
        self.body = json.dumps(self.record)
        self.status = {'id': 10, 'context': 'release-ready', 'sha': sha, 'state': 'success', 'creator': OWNER,
                       'target_url': 'https://github.com/darkquasar/batz/issues/1#issuecomment-20',
                       'description': 'sha256:'+hashlib.sha256(self.body.encode()).hexdigest()}
        self.failed = None

    def statuses(self, sha):
        return [self.status]

    def comment(self, identity):
        return {'id': identity, 'user': OWNER, 'body': self.body,
                'html_url': self.status['target_url'],
                'issue_url': 'https://api.github.com/repos/darkquasar/batz/issues/1'}

    def run(self, identity, attempt):
        return {'id': identity, 'head_sha': self.sha, 'status': 'completed', 'conclusion': 'success',
                'repository': {'full_name': 'darkquasar/batz'}, 'head_repository': {'full_name': 'darkquasar/batz'},
                'path': '.github/workflows/release.yml@refs/tags/v1.2.3', 'event': 'push', 'run_attempt': attempt,
                'check_suite_id': identity, 'actor': OWNER, 'triggering_actor': OWNER}

    def check(self, identity):
        lane = next(row['lane'] for row in self.record['checks'] if row['check_id'] == identity)
        return {'id': identity, 'head_sha': self.sha, 'status': 'completed',
                'conclusion': 'failure' if lane == self.failed else 'success',
                'name': release.JOBS[lane], 'app': {'id': 15368}, 'check_suite': {'id': identity}}

    def jobs(self, identity, attempt):
        check = self.check(identity)
        return [{'id': identity, 'name': check['name'], 'head_sha': self.sha, 'run_id': identity,
                 'check_run_url': f'https://api.github.com/repos/darkquasar/batz/check-runs/{identity}',
                 'status': 'completed', 'conclusion': check['conclusion'],
                 'steps': [{'name': release.STEPS[check['name']] +
                            (' [sha256:' + self.record['candidate_sha256'] + ']' if check['name'].startswith('install-') else ''),
                            'status': 'completed', 'conclusion': 'success'}]}]


class Provenance(unittest.TestCase):
    def test_failed_actual_check_cannot_be_owner_reported_as_passed(self):
        github = ProofGitHub()
        github.failed = 'app'
        with self.assertRaisesRegex(ValueError, 'check'):
            release.readiness(github, github.sha)

class Registry:
    def verify(self, candidate, env):
        if not env.get('BATZ_RELEASE_IMAGE'):
            raise ValueError('explicit registry configuration required')


class PublicationGitHub(ProofGitHub):
    def __init__(self, sha):
        super().__init__(sha)
        self.remote = None
        self.assets = {}
        self.interrupt = True

    def release(self, tag):
        return copy.deepcopy(self.remote)

    def create(self, tag, sha, body):
        self.remote = {'id': 30, 'tag_name': tag, 'target_commitish': sha, 'body': body, 'draft': True, 'prerelease': '-' in tag}
        return self.release(tag)

    def release_assets(self, identity):
        return [{'id': name, 'name': name} for name in self.assets]

    def download(self, identity):
        return self.assets[identity]

    def upload(self, identity, name, data):
        if name in self.assets:
            raise ValueError('duplicate remote asset')
        if self.interrupt and self.assets:
            self.interrupt = False
            raise OSError('invented upload interruption')
        self.assets[name] = data

    def complete(self, identity):
        self.remote['draft'] = False
        return copy.deepcopy(self.remote)


class PublicationFixture(GitFixture):
    def setUp(self):
        super().setUp()
        self.dist = self.root / 'dist'; self.dist.mkdir()
        self.github = PublicationGitHub(self.sha)
        self.env['BATZ_RELEASE_IMAGE'] = 'registry.invalid/owner/batz'
        # Small real tar files exercise the actual A2 producer/consumer contract.
        import io
        import tarfile
        build_spec = importlib.util.spec_from_file_location('build', Path(__file__).resolve().parents[1]/'packaging/build.py')
        build = importlib.util.module_from_spec(build_spec); build_spec.loader.exec_module(build)
        rows = []
        for target in sorted(release.TARGETS):
            files = {name: b'invented' for name in build.REQUIRED + build.BINARY_PATHS}
            files.update({'models/example.sql': b'SELECT 1', 'pipelines/example.yaml': b'invented', 'schemas/example.json': b'{}'})
            content = {'schema': build.SCHEMA, 'version': 'v1.2.3', 'source_sha': self.sha,
                       'target': target, 'development': False, 'dirty': False, 'source_tree_sha256': 'c'*64,
                       'files': {name: release.digest(data) for name, data in files.items()}}
            files['release-content.json'] = release.json_bytes(content)
            top = 'batz-v1.2.3-' + target.replace('/', '-')
            archive = self.dist / (top+'.tar.gz')
            with tarfile.open(archive, 'w:gz') as output:
                for name, data in files.items():
                    entry = tarfile.TarInfo(top+'/'+name); entry.size = len(data)
                    output.addfile(entry, io.BytesIO(data))
            row, _ = build.inspect_archive(archive); rows.append(row)
        candidate = {'schema': 'batz-release-candidate/v0alpha1', 'version': 'v1.2.3', 'source_sha': self.sha,
                     'development': False, 'dirty': False, 'source_tree_sha256': 'c'*64, 'targets': rows,
                     'docker': {'target': 'linux/amd64', 'digest': 'sha256:'+'f'*64, 'version': 'v1.2.3', 'source_sha': self.sha}}
        self.candidate = self.dist/'candidate.json'; self.candidate.write_bytes(release.json_bytes(candidate))
        self.github.record['candidate_sha256'] = release.digest(self.candidate.read_bytes())
        self.github.record['content_sha256'] = {row['target']: row['content_sha256'] for row in rows}
        for check in self.github.record['checks']:
            check['candidate_sha256'] = self.github.record['candidate_sha256']
        self.github.body = json.dumps(self.github.record)
        self.github.status['description'] = 'sha256:'+release.digest(self.github.body.encode())
        self.manifest = release.seal(self.candidate, self.dist, self.github)



class ProvenanceRegressions(unittest.TestCase):
    def test_required_proof_cannot_be_missing_skipped_stale_or_mutated(self):
        for bad in ('latest-status', 'status-sha', 'body', 'review', 'missing', 'source', 'inputs', 'skipped-step', 'candidate-step', 'check-app', 'run-actor'):
            github = ProofGitHub()
            if bad == 'latest-status':
                old = copy.deepcopy(github.status); github.status = dict(old, id=11, state='pending')
                github.statuses = lambda sha: [old, github.status]
            elif bad == 'status-sha':
                github.status['sha'] = 'f'*40
            elif bad == 'body':
                github.body += ' '
            elif bad == 'review':
                github.record['review']['disposition'] = 'unresolved'
            elif bad == 'missing':
                github.record['checks'].pop()
            elif bad == 'source':
                github.sha = '9'*40
            elif bad == 'inputs':
                github.record['checks'][0]['candidate_sha256'] = '9'*64
            elif bad == 'skipped-step':
                original = github.jobs
                def skipped(identity, attempt):
                    rows = original(identity, attempt); rows[0]['steps'][0]['conclusion'] = 'skipped'; return rows
                github.jobs = skipped
            elif bad == 'candidate-step':
                original = github.jobs
                def substituted(identity, attempt):
                    rows = original(identity, attempt)
                    rows[0]['steps'][0]['name'] = rows[0]['steps'][0]['name'].replace('b'*64, '9'*64)
                    return rows
                github.jobs = substituted
            elif bad == 'check-app':
                original = github.check
                def foreign(identity):
                    row = original(identity); row['app']['id'] = 9; return row
                github.check = foreign
            elif bad == 'run-actor':
                original = github.run
                def foreign(identity, attempt):
                    row = original(identity, attempt); row['triggering_actor'] = {'id': 9}; return row
                github.run = foreign
            if bad in ('review', 'missing', 'inputs'):
                github.body = json.dumps(github.record)
                github.status['description'] = 'sha256:'+release.digest(github.body.encode())
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                release.readiness(github, 'a'*40)

    def test_completed_prior_attempt_can_qualify_publisher_only_retry(self):
        github = ProofGitHub()
        original_run, original_jobs = github.run, github.jobs
        def prior_run(identity, attempt):
            self.assertEqual(attempt, 1)
            row = original_run(identity, attempt); row['conclusion'] = 'failure'; return row
        def prior_jobs(identity, attempt):
            return original_jobs(identity, attempt) + [{'name': 'publish', 'status': 'completed', 'conclusion': 'failure'}]
        github.run, github.jobs = prior_run, prior_jobs
        record, refs = release.readiness(github, github.sha)
        self.assertEqual(refs['runs'][0]['attempt'], 1)
        def cancelled(identity, attempt):
            return original_jobs(identity, attempt) + [{'name': 'unrelated', 'status': 'completed', 'conclusion': 'cancelled'}]
        github.jobs = cancelled
        with self.assertRaisesRegex(ValueError, 'non-publisher'):
            release.readiness(github, github.sha)


class Publication(PublicationFixture):
    def test_interrupted_draft_resumes_and_identical_complete_retry_is_safe(self):
        with self.assertRaisesRegex(OSError, 'interruption'):
            release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertTrue(self.github.remote['draft'])
        result = release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertEqual(result['phase'], 'complete')
        before = copy.deepcopy(self.github.assets)
        result = release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertEqual(result['phase'], 'already-complete')
        self.assertEqual(self.github.assets, before)
    def test_corrupt_missing_duplicate_or_dirty_set_refuses_before_writes(self):
        original_candidate = self.candidate.read_bytes()
        candidate = json.loads(original_candidate)
        archive = self.dist/candidate['targets'][0]['archive']; original_archive = archive.read_bytes()
        cases = ('archive', 'missing', 'checksums', 'duplicate', 'development', 'schema')
        for bad in cases:
            if bad == 'archive': archive.write_bytes(original_archive+b'corrupt')
            elif bad == 'missing': archive.unlink()
            elif bad == 'checksums': (self.dist/'checksums.txt').write_text('')
            else:
                changed = copy.deepcopy(candidate)
                if bad == 'duplicate': changed['targets'][1] = changed['targets'][0]
                if bad == 'development': changed['development'] = True
                if bad == 'schema': changed['schema'] = 'unknown'
                self.candidate.write_bytes(release.json_bytes(changed))
            with self.subTest(bad=bad), self.assertRaises(ValueError):
                release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
            self.assertIsNone(self.github.remote)
            archive.write_bytes(original_archive); self.candidate.write_bytes(original_candidate)
            if bad == 'checksums':
                data = {p.name: p.read_bytes() for p in self.dist.iterdir() if p.name != 'checksums.txt'}
                (self.dist/'checksums.txt').write_bytes(''.join(f'{release.digest(raw)}  {name}\n' for name, raw in sorted(data.items())).encode())

    def test_moved_tag_and_conflicting_remote_bytes_preserve_draft(self):
        original_upload = self.github.upload
        self.github.interrupt = False
        def move(identity, name, data):
            original_upload(identity, name, data)
            (self.root/'input').write_text('moved'); self.git('commit', '-qam', 'moved'); self.git('tag', '-f', 'v1.2.3')
        self.github.upload = move
        with self.assertRaisesRegex(ValueError, 'tag target changed'):
            release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertTrue(self.github.remote['draft'])
        self.git('tag', '-f', 'v1.2.3', self.sha); self.git('checkout', '--detach', self.sha)
        self.github.upload = original_upload
        key = next(iter(self.github.assets)); self.github.assets[key] = b'foreign bytes'
        before = copy.deepcopy(self.github.assets)
        with self.assertRaisesRegex(ValueError, 'downloaded asset'):
            release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertEqual(self.github.assets, before)
        self.assertTrue(self.github.remote['draft'])

    def test_completed_conflict_and_unconfigured_registry_never_overwrite(self):
        with self.assertRaisesRegex(ValueError, 'registry'):
            release.publish(self.manifest, self.dist, self.root, dict(self.env, BATZ_RELEASE_IMAGE=''), self.github, Registry())
        self.assertIsNone(self.github.remote)
        self.github.interrupt = False
        release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        key = next(iter(self.github.assets)); self.github.assets[key] = b'conflicting complete bytes'
        before = copy.deepcopy(self.github.assets)
        with self.assertRaisesRegex(ValueError, 'downloaded asset'):
            release.publish(self.manifest, self.dist, self.root, self.env, self.github, Registry())
        self.assertEqual(self.github.assets, before)
        self.assertFalse(self.github.remote['draft'])


class Commands(unittest.TestCase):
    def test_missing_registry_selection_refuses_without_docker(self):
        with self.assertRaisesRegex(ValueError, 'configuration required'):
            release.Registry().verify({}, {})

    def test_cli_foreign_repository_refuses_without_external_access(self):
        command = [os.sys.executable, '-I', '-B', str(Path(__file__).with_name('release.py')), 'authorize']
        result = subprocess.run(command, env={'PATH': '/usr/bin:/bin', 'GITHUB_REPOSITORY': 'invented/batz'},
                                text=True, capture_output=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn('repository must be darkquasar/batz', result.stderr)


if __name__ == '__main__':
    unittest.main()
