#!/usr/bin/env python3
"""Authorize and publish one qualified, immutable Batz release set.

GitHub and the selected registry are external boundaries. No command creates tags,
statuses, readiness comments or images. Publication accepts already qualified bytes.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import urllib.error
import urllib.parse
import urllib.request

REPOSITORY = 'darkquasar/batz'
OWNER_ID = 28435534
TARGETS = {'darwin/arm64', 'linux/amd64'}
SHA = re.compile(r'[0-9a-f]{40}\Z')
DIGEST = re.compile(r'[0-9a-f]{64}\Z')
VERSION = re.compile(r'v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?\Z')
LANES = {'fast', 'app', 'native', 'adapters', 'install-darwin-arm64', 'install-linux-amd64', 'install-docker-linux-amd64'}


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def json_bytes(value):
    return (json.dumps(value, sort_keys=True, indent=2) + '\n').encode()


def owner(user):
    return user.get('id') == OWNER_ID and user.get('login') == 'darkquasar'


def version(value):
    match = VERSION.fullmatch(value)
    require(match is not None and not (match.group(4) and any(
        part.isdigit() and len(part) > 1 and part.startswith('0') for part in match.group(4).split('.'))),
        'version must be strict v-prefixed SemVer without metadata')
    return value


def git(root, *args):
    return subprocess.check_output(['git', *args], cwd=root, text=True, stderr=subprocess.PIPE).strip()


def authorize(root, env, github):
    require(env.get('GITHUB_REPOSITORY') == REPOSITORY, 'repository must be ' + REPOSITORY)
    require(env.get('GITHUB_EVENT_NAME') == 'push', 'release requires a push tag event')
    ref = env.get('GITHUB_REF', '')
    require(ref.startswith('refs/tags/'), 'release requires a tag ref')
    tag = version(ref.removeprefix('refs/tags/'))
    require(env.get('GITHUB_ACTOR') == 'darkquasar' and env.get('GITHUB_ACTOR_ID') == str(OWNER_ID),
            'only the owner may create release tags')
    require(env.get('GITHUB_TRIGGERING_ACTOR') == 'darkquasar' and
            owner(github.user(env.get('GITHUB_TRIGGERING_ACTOR', ''))), 'only the owner may rerun releases')
    event = json.loads(Path(env['GITHUB_EVENT_PATH']).read_bytes())
    require(event.get('repository', {}).get('full_name') == REPOSITORY and owner(event.get('sender', {})) and
            event.get('ref') == ref and event.get('deleted') is False and not event.get('forced'),
            'owner tag push event identity mismatch')
    sha = env.get('GITHUB_SHA', '')
    require(SHA.fullmatch(sha) is not None, 'full source SHA is required')
    # Fetch explicit remote refs each time, rather than trusting a cached main/tag.
    git(root, 'fetch', '--no-tags', 'origin', 'refs/heads/main')
    main = git(root, 'rev-parse', 'FETCH_HEAD^{commit}')
    git(root, 'fetch', '--no-tags', 'origin', ref)
    peeled = git(root, 'rev-parse', 'FETCH_HEAD^{commit}')
    require(peeled == sha, 'tag target changed or does not match exact source SHA')
    require(git(root, 'rev-parse', 'HEAD') == sha, 'checkout does not match exact tagged source')
    try:
        git(root, 'merge-base', '--is-ancestor', sha, main)
    except subprocess.CalledProcessError as error:
        raise ValueError('tag is outside freshly fetched main history') from error
    # Admission precedes qualification. Exact-SHA readiness and candidate bytes
    # are required by seal/verify_set and rechecked before every publication write.
    return {'version': tag, 'source_sha': sha, 'ref': ref, 'actor_id': OWNER_ID, 'rerun_actor_id': OWNER_ID}


JOBS = {'fast': 'fast', 'app': 'shared-runtime', 'native': 'models', 'adapters': 'adapters',
        'install-darwin-arm64': 'install-darwin-arm64', 'install-linux-amd64': 'install-linux-amd64',
        'install-docker-linux-amd64': 'install-docker-linux-amd64'}
STEPS = {'fast': 'Full fast contracts', 'shared-runtime': 'Full shared app contracts',
         'models': 'Full native catalog contracts', 'adapters': 'Full adapter catalog contracts',
         'install-darwin-arm64': 'Qualify installed darwin/arm64 candidate',
         'install-linux-amd64': 'Qualify installed linux/amd64 candidate',
         'install-docker-linux-amd64': 'Qualify installed Docker linux/amd64 candidate'}


def readiness(github, sha):
    statuses = [row for row in github.statuses(sha) if row.get('context') == 'release-ready']
    require(bool(statuses), 'missing release-ready status')
    status = max(statuses, key=lambda row: row['id'])
    require(status.get('sha') == sha and status.get('state') == 'success' and owner(status.get('creator', {})),
            'latest release-ready status must succeed on exact SHA with owner creator')
    match = re.fullmatch(r'https://github\.com/darkquasar/batz/issues/([1-9][0-9]*)#issuecomment-([1-9][0-9]*)',
                         status.get('target_url', ''))
    require(match is not None, 'readiness must link an owner Batz issue comment')
    comment = github.comment(int(match.group(2)))
    require(comment.get('id') == int(match.group(2)) and owner(comment.get('user', {})) and
            comment.get('html_url') == status['target_url'] and
            comment.get('issue_url') == f'https://api.github.com/repos/{REPOSITORY}/issues/{match.group(1)}',
            'readiness comment provenance mismatch')
    body = comment['body'].encode()
    body_digest = digest(body)
    require(status.get('description') == 'sha256:' + body_digest, 'readiness body digest changed')
    record = json.loads(body)
    require(record.get('schema') == 'batz-release-readiness/v0alpha1' and record.get('source_sha') == sha,
            'readiness schema/source mismatch')
    for key in ('candidate_sha256', 'source_tree_sha256'):
        require(DIGEST.fullmatch(record.get(key, '')) is not None, 'invalid readiness ' + key)
    require(set(record.get('content_sha256', {})) == TARGETS and
            all(DIGEST.fullmatch(value) for value in record['content_sha256'].values()), 'readiness content mismatch')
    require(re.fullmatch(r'sha256:[0-9a-f]{64}', record.get('docker_digest', '')) is not None, 'readiness image missing')
    review = record.get('review', {})
    require(review.get('disposition') == 'accepted' and DIGEST.fullmatch(review.get('report_sha256', '')),
            'accepted aggregate R review disposition required')
    checks = record.get('checks', [])
    require(len(checks) == len(LANES) and {row.get('lane') for row in checks} == LANES,
            'missing or duplicate required check lanes')
    require(len({row['check_id'] for row in checks}) == len(checks), 'duplicate check identities')
    attempts = {}
    for proof in checks:
        require(proof.get('source_tree_sha256') == record['source_tree_sha256'] and
                proof.get('candidate_sha256') == record['candidate_sha256'], 'check inputs mismatch')
        key = proof['run_id'], proof['run_attempt']
        if key not in attempts:
            attempts[key] = (github.run(*key), github.jobs(*key))
        run, attempt_jobs = attempts[key]
        require(run.get('id') == proof['run_id'] and run.get('head_sha') == sha and
                run.get('repository', {}).get('full_name') == REPOSITORY and
                run.get('head_repository', {}).get('full_name') == REPOSITORY and
                run.get('run_attempt') == proof['run_attempt'] and run.get('status') == 'completed' and
                run.get('conclusion') in ('success', 'failure') and
                run.get('path', '').partition('@')[0] in ('.github/workflows/release.yml', '.github/workflows/pr.yml') and
                run.get('event') in ('push', 'pull_request'), 'required workflow run identity/outcome mismatch')
        if run['event'] == 'push':
            require(owner(run.get('actor', {})) and owner(run.get('triggering_actor', {})), 'workflow run actor mismatch')
        require(attempt_jobs and all(job.get('status') == 'completed' and
                (job.get('conclusion') == 'success' or
                 (job.get('name') == 'publish' and job.get('conclusion') == 'failure')) for job in attempt_jobs),
                'check workflow attempt has failed/skipped/cancelled non-publisher work')
        if run['conclusion'] == 'failure':
            require(any(job.get('name') == 'publish' and job.get('conclusion') == 'failure' for job in attempt_jobs),
                    'failed workflow attempt lacks publisher-only failure')
        check = github.check(proof['check_id'])
        job_name = JOBS[proof['lane']]
        require(check.get('id') == proof['check_id'] and check.get('head_sha') == sha and
                check.get('status') == 'completed' and check.get('conclusion') == 'success' and
                check.get('app', {}).get('id') == 15368 and check.get('name') == job_name and
                check.get('check_suite', {}).get('id') == run.get('check_suite_id'),
                'required check identity/outcome mismatch')
        jobs = [job for job in attempt_jobs if
                job.get('check_run_url') == f'https://api.github.com/repos/{REPOSITORY}/check-runs/{check["id"]}']
        require(len(jobs) == 1, 'required check is missing from bound run attempt')
        job = jobs[0]
        require(job.get('name') == job_name and job.get('head_sha') == sha and job.get('run_id') == run['id'] and
                job.get('status') == 'completed' and
                job.get('conclusion') == 'success', 'required job identity/outcome mismatch')
        step_name = STEPS[job_name]
        if proof['lane'].startswith('install-'):
            step_name += ' [sha256:' + record['candidate_sha256'] + ']'
        executed = [step for step in job.get('steps', []) if step.get('name') == step_name]
        require(len(executed) == 1 and executed[0].get('status') == 'completed' and
                executed[0].get('conclusion') == 'success', 'required check execution missing/skipped/cancelled')
    refs = {'status_id': status['id'], 'comment_id': comment['id'], 'body_sha256': body_digest,
            'runs': [{'id': identity, 'attempt': attempt} for identity, attempt in
                     sorted({(row['run_id'], row['run_attempt']) for row in checks})], 'check_ids': sorted(row['check_id'] for row in checks)}
    return record, refs


def package_inspector():
    spec = importlib.util.spec_from_file_location('batz_package', Path(__file__).resolve().parents[1] / 'packaging/build.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module.inspect_archive


def asset_path(dist, name):
    require(isinstance(name, str) and name and Path(name).name == name and name not in ('.', '..'), 'invalid asset name')
    path = Path(dist) / name
    require(path.is_file() and not path.is_symlink(), 'missing or symlink release asset: ' + name)
    return path


def validate_candidate(candidate_path, dist):
    candidate_path = Path(candidate_path)
    require(candidate_path.resolve().parent == Path(dist).resolve(), 'candidate must be in DIST')
    candidate_bytes = asset_path(dist, candidate_path.name).read_bytes()
    candidate = json.loads(candidate_bytes)
    require(candidate.get('schema') == 'batz-release-candidate/v0alpha1', 'unsupported candidate schema')
    version(candidate['version'])
    require(SHA.fullmatch(candidate['source_sha']) and DIGEST.fullmatch(candidate['source_tree_sha256']), 'invalid candidate source')
    require(candidate.get('development') is False and candidate.get('dirty') is False, 'development/dirty candidate cannot publish')
    rows = candidate['targets']
    require(len(rows) == 2 and {row['target'] for row in rows} == TARGETS, 'missing or duplicate native targets')
    assets = {candidate_path.name: candidate_bytes}
    inspect = package_inspector()
    for row in rows:
        name = row['archive']
        require(name not in assets and name not in ('release.json', 'checksums.txt'), 'duplicate asset identity')
        path = asset_path(dist, name)
        actual, content = inspect(path)
        require(actual == row, 'archive inventory/digest mismatch')
        require(all(content.get(key) == candidate.get(key) for key in
                    ('version', 'source_sha', 'development', 'dirty', 'source_tree_sha256')), 'archive source identity mismatch')
        assets[name] = path.read_bytes()
    image = candidate['docker']
    require(image.get('target') == 'linux/amd64' and image.get('version') == candidate['version'] and
            image.get('source_sha') == candidate['source_sha'] and
            re.fullmatch(r'sha256:[0-9a-f]{64}', image.get('digest', '')), 'immutable image identity mismatch')
    return candidate, assets


def verify_set(manifest, dist, github):
    manifest = Path(manifest)
    require(manifest.resolve().parent == Path(dist).resolve(), 'final manifest must be in DIST')
    final_bytes = asset_path(dist, manifest.name).read_bytes()
    final = json.loads(final_bytes)
    require(final.get('schema') == 'batz-release/v0alpha1', 'unsupported final manifest schema')
    candidate_ref = final['candidate']
    candidate, assets = validate_candidate(asset_path(dist, candidate_ref['name']), dist)
    require(digest(assets[candidate_ref['name']]) == candidate_ref['sha256'], 'candidate manifest digest mismatch')
    require(final.get('version') == candidate['version'] and final.get('source_sha') == candidate['source_sha'], 'final identity mismatch')
    require(manifest.name not in assets and manifest.name != 'checksums.txt', 'duplicate manifest identity')
    assets[manifest.name] = final_bytes
    record, refs = readiness(github, candidate['source_sha'])
    require(record['candidate_sha256'] == candidate_ref['sha256'] and
            record['source_tree_sha256'] == candidate['source_tree_sha256'] and
            record['content_sha256'] == {row['target']: row['content_sha256'] for row in candidate['targets']} and
            record['docker_digest'] == candidate['docker']['digest'], 'readiness does not qualify these exact candidate bytes')
    require(final.get('readiness') == refs, 'final readiness references changed')
    checksums = ''.join(f'{digest(data)}  {name}\n' for name, data in sorted(assets.items())).encode()
    require(asset_path(dist, 'checksums.txt').read_bytes() == checksums, 'checksums inventory/digest mismatch')
    assets['checksums.txt'] = checksums
    require({path.name for path in Path(dist).iterdir()} == set(assets), 'unexpected/duplicate release assets in DIST')
    return final, candidate, assets


def seal(candidate_path, dist, github):
    candidate_path = Path(candidate_path)
    require(candidate_path.resolve().parent == Path(dist).resolve(), 'candidate must be in DIST')
    candidate_bytes = asset_path(dist, candidate_path.name).read_bytes()
    candidate = json.loads(candidate_bytes)
    record, refs = readiness(github, candidate['source_sha'])
    require(record['candidate_sha256'] == digest(candidate_bytes), 'readiness candidate digest mismatch')
    final = {'schema': 'batz-release/v0alpha1', 'version': candidate['version'], 'source_sha': candidate['source_sha'],
             'candidate': {'name': candidate_path.name, 'sha256': digest(candidate_bytes)}, 'readiness': refs}
    final_path = Path(dist) / 'release.json'
    names = [candidate_path.name, *(row['archive'] for row in candidate['targets'])]
    require(len(set(names + ['release.json', 'checksums.txt'])) == 5, 'duplicate asset identities')
    # Exclusive local creation also makes accidental re-sealing visible.
    with final_path.open('xb') as output:
        output.write(json_bytes(final))
    assets = {name: asset_path(dist, name).read_bytes() for name in names + ['release.json']}
    with (Path(dist) / 'checksums.txt').open('xb') as output:
        output.write(''.join(f'{digest(data)}  {name}\n' for name, data in sorted(assets.items())).encode())
    verify_set(final_path, dist, github)
    return final_path


def publish(manifest, dist, root, env, github, registry):
    final, candidate, assets = verify_set(manifest, dist, github)

    def guard():
        authorization = authorize(root, env, github)
        require(authorization['source_sha'] == candidate['source_sha'] and authorization['version'] == candidate['version'],
                'event/candidate identity mismatch')
        # Readiness revocation and changed local bytes cannot slip past qualification.
        require(verify_set(manifest, dist, github)[2] == assets, 'release bytes changed before write')

    guard()
    registry.verify(candidate, env)
    expected_body = ('Batz release set sha256:' + digest(assets[Path(manifest).name]) + '\n\n'
                     'Local-file CloudTrail and existing single-binding behavior. '
                     'Live acquisition, R2, GCP, Workspace and future S8 capabilities remain deferred.')

    def matching(remote):
        require(remote is not None and remote.get('tag_name') == candidate['version'] and
                remote.get('target_commitish') == candidate['source_sha'] and remote.get('body') == expected_body and
                remote.get('prerelease') == ('-' in candidate['version']), 'existing release conflicts with this candidate')

    def downloaded(remote, complete=False):
        matching(remote)
        inventory = github.release_assets(remote['id'])
        names = [row['name'] for row in inventory]
        require(len(names) == len(set(names)) and set(names) <= set(assets), 'conflicting or duplicate remote assets')
        for row in inventory:
            require(github.download(row['id']) == assets[row['name']], 'downloaded asset digest mismatch: ' + row['name'])
        if complete:
            require(set(names) == set(assets), 'incomplete remote release set')
        return set(names)

    remote = github.release(candidate['version'])
    if remote is None:
        guard()
        remote = github.create(candidate['version'], candidate['source_sha'], expected_body)
    names = downloaded(remote, complete=not remote['draft'])
    if not remote['draft']:
        return {'phase': 'already-complete', 'release_id': remote['id']}
    identity = remote['id']
    for name, data in assets.items():
        if name in names:
            continue
        guard()
        remote = github.release(candidate['version'])
        require(remote['id'] == identity and remote['draft'], 'release changed during upload')
        names = downloaded(remote)
        if name not in names:
            github.upload(identity, name, data)
    downloaded(github.release(candidate['version']), complete=True)
    guard()
    remote = github.release(candidate['version'])
    require(remote['id'] == identity and remote['draft'], 'release changed before completion')
    downloaded(remote, complete=True)
    completed = github.complete(identity)
    require(not completed['draft'], 'release did not become public')
    downloaded(github.release(candidate['version']), complete=True)
    return {'phase': 'complete', 'release_id': identity}


class GitHub:
    """Small REST adapter. Public reads work without any publication credential."""
    def __init__(self, token=None):
        self.token = token

    def request(self, path, method='GET', data=None, binary=False, upload=False):
        host = 'uploads.github.com' if upload else 'api.github.com'
        url = 'https://' + host + '/' + path.lstrip('/')
        headers = {'Accept': 'application/octet-stream' if binary else 'application/vnd.github+json',
                   'X-GitHub-Api-Version': '2022-11-28', 'User-Agent': 'batz-release'}
        if self.token:
            headers['Authorization'] = 'Bearer ' + self.token
        if data is not None:
            headers['Content-Type'] = 'application/octet-stream' if upload else 'application/json'
            if not upload:
                data = json_bytes(data)
        class SafeRedirect(urllib.request.HTTPRedirectHandler):
            def redirect_request(self, req, fp, code, msg, hdrs, newurl):
                redirected = super().redirect_request(req, fp, code, msg, hdrs, newurl)
                require(urllib.parse.urlsplit(newurl).scheme == 'https', 'refusing non-HTTPS API redirect')
                if urllib.parse.urlsplit(newurl).netloc != urllib.parse.urlsplit(req.full_url).netloc:
                    redirected.remove_header('Authorization')
                return redirected
        request = urllib.request.Request(url, data=data, method=method, headers=headers)
        with urllib.request.build_opener(SafeRedirect).open(request, timeout=120) as response:
            raw = response.read()
        return raw if binary else json.loads(raw)

    def pages(self, path, key=None):
        rows = []
        for page in range(1, 1001):
            result = self.request(path + ('&' if '?' in path else '?') + f'per_page=100&page={page}')
            items = result[key] if key else result
            rows.extend(items)
            if len(items) < 100:
                return rows
        raise ValueError('GitHub pagination limit exceeded')

    def user(self, login):
        return self.request('users/' + urllib.parse.quote(login, safe=''))

    def statuses(self, sha):
        combined = self.request(f'repos/{REPOSITORY}/commits/{sha}/status')
        require(combined.get('sha') == sha, 'GitHub status endpoint SHA mismatch')
        return [dict(row, sha=sha) for row in self.pages(f'repos/{REPOSITORY}/commits/{sha}/statuses')]

    def comment(self, identity):
        return self.request(f'repos/{REPOSITORY}/issues/comments/{int(identity)}')

    def run(self, identity, attempt):
        return self.request(f'repos/{REPOSITORY}/actions/runs/{int(identity)}/attempts/{int(attempt)}')

    def check(self, identity):
        return self.request(f'repos/{REPOSITORY}/check-runs/{int(identity)}')

    def jobs(self, identity, attempt):
        return self.pages(f'repos/{REPOSITORY}/actions/runs/{int(identity)}/attempts/{int(attempt)}/jobs', 'jobs')

    def release(self, tag):
        # Listing includes drafts for the authorized final publisher. A public
        # tag endpoint alone cannot discover an interrupted draft safely.
        matches = [row for row in self.pages(f'repos/{REPOSITORY}/releases') if row.get('tag_name') == tag]
        require(len(matches) <= 1, 'duplicate releases for this version')
        return matches[0] if matches else None

    def create(self, tag, sha, body):
        return self.request(f'repos/{REPOSITORY}/releases', 'POST',
                            {'tag_name': tag, 'target_commitish': sha, 'name': tag, 'body': body,
                             'draft': True, 'prerelease': '-' in tag, 'make_latest': 'false'})

    def release_assets(self, identity):
        return self.pages(f'repos/{REPOSITORY}/releases/{int(identity)}/assets')

    def download(self, identity):
        return self.request(f'repos/{REPOSITORY}/releases/assets/{int(identity)}', binary=True)

    def upload(self, identity, name, data):
        return self.request(f'repos/{REPOSITORY}/releases/{int(identity)}/assets?name=' + urllib.parse.quote(name, safe=''),
                            'POST', data, upload=True)

    def complete(self, identity):
        return self.request(f'repos/{REPOSITORY}/releases/{int(identity)}', 'PATCH', {'draft': False, 'make_latest': 'false'})


class Registry:
    """Verify an already staged image; D8 must select the namespace/access first.

    Image creation/push/tagging is deliberately not inferred by this publisher.
    """
    def verify(self, candidate, env):
        image = env.get('BATZ_RELEASE_IMAGE', '')
        require(re.fullmatch(r'[a-z0-9.-]+(?::[0-9]+)?/[a-z0-9._/-]+', image) is not None and
                '..' not in image and '//' not in image, 'explicit Docker registry/repository configuration required (D8)')
        reference = image + '@' + candidate['docker']['digest']
        subprocess.run(['docker', 'pull', '--platform', 'linux/amd64', reference], check=True, timeout=600)
        rows = json.loads(subprocess.check_output(['docker', 'image', 'inspect', reference], text=True, timeout=60))
        require(len(rows) == 1, 'image inspection is ambiguous')
        row = rows[0]
        labels = row.get('Config', {}).get('Labels') or {}
        require(reference in row.get('RepoDigests', []) and row.get('Os') == 'linux' and row.get('Architecture') == 'amd64' and
                labels.get('org.opencontainers.image.version') == candidate['version'] and
                labels.get('org.opencontainers.image.revision') == candidate['source_sha'] and
                labels.get('org.opencontainers.image.batz-candidate-tree') == candidate['source_tree_sha256'],
                'registry image digest/platform/source identity mismatch')
        return reference


def fetch_candidate(url, expected_digest, dist, env):
    require(DIGEST.fullmatch(expected_digest) is not None, 'configured frozen candidate SHA256 is required')
    parsed = urllib.parse.urlsplit(url)
    require(parsed.scheme == 'https' and parsed.netloc and not parsed.username and not parsed.password and
            not parsed.query and not parsed.fragment, 'candidate URL must be explicit HTTPS without credentials/query/fragment')
    destination = Path(dist)
    destination.mkdir(parents=True, exist_ok=False)

    def download(source, path, maximum):
        # Candidate hosting receives no GitHub/registry credential.
        with urllib.request.urlopen(source, timeout=120) as response:
            require(urllib.parse.urlsplit(response.url).scheme == 'https', 'candidate redirect must remain HTTPS')
            data = response.read(maximum + 1)
        require(len(data) <= maximum, 'candidate download exceeds declared limit')
        with path.open('xb') as output:
            output.write(data)
        return data

    raw = download(url, destination / 'candidate.json', 1024 * 1024)
    require(digest(raw) == expected_digest, 'downloaded frozen candidate digest mismatch')
    candidate = json.loads(raw)
    require(candidate.get('version') == env.get('GITHUB_REF', '').removeprefix('refs/tags/') and
            candidate.get('source_sha') == env.get('GITHUB_SHA'), 'downloaded candidate does not match event')
    names = [row['archive'] for row in candidate['targets']]
    require(len(names) == 2 and len(set(names)) == 2, 'candidate archive identities duplicate/missing')
    for name in names:
        require(Path(name).name == name and re.fullmatch(r'batz-v[0-9A-Za-z.-]+-(darwin-arm64|linux-amd64)\.tar\.gz', name),
                'invalid candidate archive filename')
        download(urllib.parse.urljoin(url, name), destination / name, 512 * 1024 * 1024)
    validate_candidate(destination / 'candidate.json', destination)
    return destination / 'candidate.json'


def qualify_native(candidate_path, dist, target, env, prepared_install=None):
    """Run the documented outside-Git path on the actual target, never emulation."""
    import platform
    import shutil
    import tarfile
    import tempfile
    native = {('Darwin', 'arm64'): 'darwin/arm64', ('Linux', 'x86_64'): 'linux/amd64'}.get((platform.system(), platform.machine()))
    require(native == target, 'qualification requires actual native ' + target)
    candidate, _ = validate_candidate(candidate_path, dist)
    row = next(row for row in candidate['targets'] if row['target'] == target)
    with tempfile.TemporaryDirectory(prefix='Batz release qualification ') as scratch:
        scratch = Path(scratch)
        if prepared_install is None:
            with tarfile.open(asset_path(dist, row['archive']), 'r:gz') as archive:
                archive.extractall(scratch, filter='data')  # Already inspected: only regular safe paths.
        install = Path(prepared_install).resolve() if prepared_install else scratch / row['archive'].removesuffix('.tar.gz')
        require(digest((install/'release-content.json').read_bytes()) == row['content_sha256'],
                'prepared installation differs from frozen candidate')
        caller = scratch / 'unrelated caller'; caller.mkdir()
        workspace = scratch / 'explicit workspace'
        binary = install / 'bin/batz'
        pipeline = install / 'pipelines/aws-cloudtrail/pipeline.yaml'
        run_env = {key: value for key, value in env.items() if not key.startswith(('PYTHON', 'UV_', 'PIP_', 'CONDA')) and key != 'VIRTUAL_ENV'}
        run_env.update(PYTHONDONTWRITEBYTECODE='1')
        run_env.pop('BATZ_TEST_RUNTIME', None)
        run_env.pop('BATZ_TOOLCHAIN_ROOT', None)
        def run(argv, cwd=caller):
            subprocess.run(list(map(str, argv)), cwd=cwd, env=run_env, check=True, timeout=1200)
        def identity():
            actual = json.loads(subprocess.check_output([str(binary), 'version', '--format', 'json'], cwd=caller, env=run_env))
            require(actual.get('version') == candidate['version'] and actual.get('source_sha') == candidate['source_sha'] and
                    actual.get('target') == target and actual.get('dirty') is False and actual.get('development') is False,
                    'installed CLI identity mismatch')
        identity()
        # macOS preparation exception: setup is explicit and precedes offline work.
        # Linux is supplied as a prepared image, so ordinary tests never sync.
        if target == 'darwin/arm64':
            mise = Path(env.get('BATZ_RELEASE_MISE', '')).resolve()
            lock = json.loads((install/'runtime/dependencies.lock.json').read_bytes())
            require(mise.is_file() and digest(mise.read_bytes()) == lock['mise']['artifacts'][target]['sha256'],
                    'qualification requires the pinned verified mise binary')
            run_env.update(MISE_BIN=str(mise), MISE_DATA_DIR=str(scratch/'mise/data'),
                           MISE_CACHE_DIR=str(scratch/'mise/cache'), MISE_CONFIG_DIR=str(scratch/'mise/config'),
                           MISE_STATE_DIR=str(scratch/'mise/state'))
            run([mise, 'trust'], install)
            run([mise, 'install'], install)
            run([mise, 'install'], install)
            shutil.rmtree(install / 'runtime/.local/venv')
            run([mise, 'install'], install)
        else:
            require(prepared_install is not None, 'Linux qualification requires an explicit baked candidate installation')
            run([install/'runtime/.local/venv/bin/python', '-I', '-B', install/'ci/setup.py', '--check'], install)
        run([binary, 'explain', pipeline, '--level', '2'])
        selection = [pipeline, '--workspace', workspace, '--binding-id', 'example', '--route', 'local_raw',
                     '--input', 'path=' + str(install/'pipelines/aws-cloudtrail/fixtures/wiring.json')]
        report_bytes = subprocess.check_output(list(map(str, [binary, 'run', *selection])), cwd=caller, env=run_env, timeout=1200)
        print(report_bytes.decode(), flush=True)
        report = json.loads(report_bytes)
        require(report.get('exit') == 0 and report.get('catalog'), 'installed run did not publish a reader catalog')
        reader = 'import duckdb,sys; c=duckdb.connect(); c.execute("ATTACH \'" + sys.argv[1].replace("\'", "\'\'") + "\' AS published (READ_ONLY)"); assert c.sql(\'SELECT count(*) FROM published.example.api_activity\').fetchone()[0] > 0'
        run([install/'runtime/.local/venv/bin/python', '-I', '-B', '-c', reader, report['catalog']])
        run([binary, 'retry', *selection])
        run([binary, 'explain', pipeline, '--level', '3', '--example', 'wiring', '--format', 'json', '--strict'])
        run([binary, 'status', '--workspace', workspace])
        # Immutable runtime content stays identical after setup/run/sample/retry.
        inventory = json.loads((install/'release-content.json').read_bytes())
        require(all(digest((install/name).read_bytes()) == expected for name, expected in inventory['files'].items()),
                'installed runtime content changed during qualification')
        identity()
    return {'target': target, 'candidate_sha256': digest(Path(candidate_path).read_bytes()), 'outcome': 'success'}


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    commands.add_parser('authorize')
    for command in ('verify-set', 'publish'):
        child = commands.add_parser(command)
        child.add_argument('manifest', type=Path); child.add_argument('dist', type=Path)
    child = commands.add_parser('seal')
    child.add_argument('candidate', type=Path); child.add_argument('dist', type=Path)
    child = commands.add_parser('fetch-candidate')
    child.add_argument('url'); child.add_argument('sha256'); child.add_argument('dist', type=Path)
    child = commands.add_parser('qualify-native')
    child.add_argument('candidate', type=Path); child.add_argument('dist', type=Path)
    child.add_argument('--target', required=True, choices=sorted(TARGETS))
    child.add_argument('--prepared-install', type=Path)
    args = parser.parse_args(argv)
    env = os.environ
    # Read-only workflow tokens authenticate API reads too. Write authority is
    # still confined to publish, which separately requires an explicit token.
    github = GitHub(env.get('GH_TOKEN'))
    root = Path(__file__).resolve().parents[1]
    if args.command == 'authorize':
        result = authorize(root, env, github)
    elif args.command == 'verify-set':
        final, _, assets = verify_set(args.manifest, args.dist, github)
        result = {'version': final['version'], 'source_sha': final['source_sha'], 'assets': sorted(assets)}
    elif args.command == 'publish':
        require(bool(env.get('GH_TOKEN')), 'final publisher requires an explicit GitHub write token')
        result = publish(args.manifest, args.dist, root, env, github, Registry())
    elif args.command == 'seal':
        result = {'manifest': str(seal(args.candidate, args.dist, github))}
    elif args.command == 'fetch-candidate':
        result = {'candidate': str(fetch_candidate(args.url, args.sha256, args.dist, env))}
    else:
        result = qualify_native(args.candidate, args.dist, args.target, env, args.prepared_install)
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (ValueError, KeyError, TypeError, OSError, subprocess.SubprocessError) as error:
        print('Batz release refused: ' + str(error), file=sys.stderr)
        sys.exit(1)
