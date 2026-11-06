# Version-tag releases

A release is a strict `vMAJOR.MINOR.PATCH` tag, optionally followed by SemVer
prerelease identifiers (`v2.0.0-rc.1`, for example). Numeric identifiers have no
leading zeros; build metadata is refused. These examples select no first version
or channel. Only `darkquasar` (GitHub account ID `28435534`) may push or rerun the
tag in `darkquasar/batz`. Publication always peels the freshly fetched remote tag,
fetches main, verifies main ancestry and checks the exact checkout/source identity.
Eligible main ancestors enter qualification before readiness is required. Every
ancestor, including a non-tip commit, needs its own complete exact-SHA qualification
before sealing or publication. Moving a tag cannot substitute another revision.

Production tags, readiness status/comments, releases and images require explicit
owner authorization. The selected candidate is the `v0.1.0-rc.2` prerelease;
the selected runtime registry is repo-linked `ghcr.io/darkquasar/batz`. Native Linux
confinement and real three-target qualification remain required; missing prepared
images or credentials fail the affected work. No performance threshold or S8
capability gates the first release. Active scope is local-file CloudTrail and the
existing single-binding runtime/reader behavior. Live acquisition, R2, GCP and
Workspace routes are deferred.

The unreleased `v0.1.0-rc.1` tag is retained unchanged after repeated anonymous
GitHub identity-lookup HTTP 403 failures. Workflow identity checks and release
API reads use the job's scoped GitHub token; read-only jobs retain read-only
permissions, and only the final publisher receives write authority. The stable
owner-ID and exact-source checks remain mandatory.

```text
+------------------+  candidate hash  +-------------------+  references  +------------------+
| Frozen candidate | ===============> | Owner readiness   | ===========> | Final manifest   |
+------------------+                  +-------------------+              +------------------+
                                                                                 | verified set
                                                                                 v
+------------------+  identical bytes +-------------------+  download    +------------------+
| Public complete  | <=============== | Verified draft    | <=========== | Uploaded draft   |
+------------------+                  +-------------------+              +------------------+
```

## Produce and qualify the candidate

Use the accepted producer, before any publication write, on clean exact source:

```sh
runtime/.local/venv/bin/python -I -B packaging/build.py archive \
  --root "$PWD" --output "$DIST" --version "$VERSION" --source-sha "$SOURCE_SHA" \
  --target darwin/arm64 --go "$PWD/runtime/.local/go/bin/go"
runtime/.local/venv/bin/python -I -B packaging/build.py archive \
  --root "$PWD" --output "$DIST" --version "$VERSION" --source-sha "$SOURCE_SHA" \
  --target linux/amd64 --go "$PWD/runtime/.local/go/bin/go"
runtime/.local/venv/bin/python -I -B packaging/build.py candidate \
  --archive "$DIST/batz-$VERSION-darwin-arm64.tar.gz" \
  --archive "$DIST/batz-$VERSION-linux-amd64.tar.gz" \
  --docker-digest "$VERIFIED_DOCKER_DIGEST" --output "$DIST/candidate.json"
```

The image digest must identify the real, separately staged Linux amd64 runtime
image in the selected registry. Cross-compilation does not qualify execution.
The producer freezes `batz-release-candidate/v0alpha1`, with exactly these fields
(the digest strings below are invented illustrations, never usable evidence):

```json
{
  "schema": "batz-release-candidate/v0alpha1",
  "version": "v2.0.0-rc.1",
  "source_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "development": false,
  "dirty": false,
  "source_tree_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
  "targets": [
    {
      "target": "darwin/arm64",
      "archive": "batz-v2.0.0-rc.1-darwin-arm64.tar.gz",
      "sha256": "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
      "bytes": 123,
      "cli_sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
      "collector_sha256": "eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee",
      "content_sha256": "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
      "runtime_lock_sha256": "1111111111111111111111111111111111111111111111111111111111111111"
    },
    {
      "target": "linux/amd64",
      "archive": "batz-v2.0.0-rc.1-linux-amd64.tar.gz",
      "sha256": "2222222222222222222222222222222222222222222222222222222222222222",
      "bytes": 456,
      "cli_sha256": "3333333333333333333333333333333333333333333333333333333333333333",
      "collector_sha256": "4444444444444444444444444444444444444444444444444444444444444444",
      "content_sha256": "5555555555555555555555555555555555555555555555555555555555555555",
      "runtime_lock_sha256": "1111111111111111111111111111111111111111111111111111111111111111"
    }
  ],
  "docker": {
    "target": "linux/amd64",
    "digest": "sha256:6666666666666666666666666666666666666666666666666666666666666666",
    "version": "v2.0.0-rc.1",
    "source_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
  }
}
```

`content_sha256` hashes the archive's exact `release-content.json` bytes; its
inventory covers the binaries, content and setup declarations. Candidate identity
also includes the reviewed source-tree digest. Development or dirty candidates
are refused by publication. Candidate archives have one installation directory,
`batz-VERSION-OS-ARCH/`. See [installation](../usage/install.md) for the user path.

Run `Prepare and qualify Linux` on the intended exact source with the selected
version. It builds both native archives, the GHCR runtime image and the separate
prepared qualification image, then retains the frozen `release-candidate` Actions
artifact. This prepares candidate bytes; the tag workflow retains publication gates.

Configure these explicit repository variables for the authorized tag run:

- `BATZ_RELEASE_CANDIDATE_RUN_ID`: Actions run holding the `release-candidate`
  artifact. The macOS job uses its read-only workflow token to download it from
  GitHub, then verifies the configured hash, full source SHA and tag version.
- `BATZ_RELEASE_CANDIDATE_SHA256`: hash of those exact candidate bytes.
- `BATZ_RELEASE_IMAGE`: `ghcr.io/darkquasar/batz`, without a tag or digest.
- `BATZ_PREPARED_LINUX_IMAGE`: full prepared-tool/fixture image reference pinned
  with `@sha256:...`, distinct from the distributed runtime image.

The release workflow always runs the full existing `ci/test fast`, `app`, native
catalog and adapter catalog commands, with no PR diff selection. macOS jobs use
the pinned preparation exception; ordinary test execution stays offline and the
existing two-slot native budget applies. The macOS installation job rebuilds both native
archives with the producer and compares their exact bytes, then uses a path with spaces
outside Git for version, fresh/repeated/repair setup, local run, retry, explain,
sample, ordinary DuckDB reader and unchanged-content checks.

The prepared Linux image interface is `/opt/batz` (exact source Git checkout and
its baked owned runtime), `/opt/release-dist` (frozen candidate and both native
archives), and `/opt/Batz installations/candidate` (separately prepared installation matching that
candidate's Linux archive inventory). It must include the pinned tools, local
fixtures, Git, Bash and Docker CLI used by the publisher. Source `ci/test all-local`
and installed probes run on native Linux amd64 with networking disabled. They do
not install/sync dependencies or fetch fixtures. Missing Linux confinement or a
failed native check blocks qualification. The distribution image similarly provides the baked
`/opt/Batz installations/candidate`; its actual Docker run executes the same installed probe.
Prepared image creation may install dependencies; ordinary qualification cannot.
Only passing exact-source native checks qualify these prepared image identities.

## Owner readiness handoff

Only after J/C/R, under a separate GitHub-write grant, the owner creates a Batz
issue comment whose **entire body is JSON**. No code fences or surrounding prose.
The schema is `batz-release-readiness/v0alpha1`. Example construction below produces
all required fields and seven checks; replace every invented identity/digest with
actual evidence before use:

```python
import hashlib, json
from pathlib import Path
candidate_bytes = Path("candidate.json").read_bytes()
candidate = json.loads(candidate_bytes)
# Each pair is the actual check-run ID and immutable workflow attempt number.
checks = {
    "fast": (101, 1), "app": (102, 1), "native": (103, 1), "adapters": (104, 1),
    "install-darwin-arm64": (105, 1), "install-linux-amd64": (106, 1),
    "install-docker-linux-amd64": (107, 1),
}
record = {
    "schema": "batz-release-readiness/v0alpha1",
    "source_sha": candidate["source_sha"],
    "candidate_sha256": hashlib.sha256(candidate_bytes).hexdigest(),
    "source_tree_sha256": candidate["source_tree_sha256"],
    "content_sha256": {row["target"]: row["content_sha256"] for row in candidate["targets"]},
    "docker_digest": candidate["docker"]["digest"],
    "review": {"disposition": "accepted", "report_sha256": "7" * 64},
    "checks": [
        {"lane": lane, "run_id": 1000, "run_attempt": attempt, "check_id": check_id,
         "source_tree_sha256": candidate["source_tree_sha256"],
         "candidate_sha256": hashlib.sha256(candidate_bytes).hexdigest()}
        for lane, (check_id, attempt) in checks.items()
    ],
}
body = json.dumps(record, sort_keys=True, indent=2) + "\n"
Path("readiness.json").write_text(body)
print("sha256:" + hashlib.sha256(body.encode()).hexdigest())
```

The latest `release-ready` commit status for the exact full source SHA must have
`state: success`, owner creator/login/stable ID, `target_url` equal to
`https://github.com/darkquasar/batz/issues/NUMBER#issuecomment-ID`, and `description`
equal to the printed `sha256:HASH`. The comment author must have that same stable
owner identity. Editing even whitespace invalidates its digest. A newer pending
or failed status revokes readiness. No private specs repository is needed.

The consumer checks the actual GitHub run attempt, source/repository, Actions app
ID `15368`, check suite, check-run ID/name, job identity and successful executed
proof step. The four source jobs are `fast`, `shared-runtime`, `models`, `adapters`.
The three install jobs have the lane names above. Their proof step names end in
`[sha256:CANDIDATE_SHA256]`, binding actual configured candidate input as well as
job success. Missing/skipped/cancelled jobs or required steps, changed candidate
inputs and failed source/platform checks are refused. Aggregate R must have an
accepted disposition with the report hash; no new S8 or cost gate is implied.

Qualification precedes publication. On an initial tag attempt, all seven required
checks and `gate` may succeed while `publish` fails solely because readiness is
absent. That completed attempt is usable evidence: the only permitted failed job
is `publish`; every other job must have succeeded. The owner can then record
readiness and rerun the failed publisher. The record binds
`actions/runs/RUN/attempts/ATTEMPT` and its jobs, so a later in-progress rerun cannot
replace those immutable outcomes. Cancelled/in-progress qualification attempts
are rejected. Publication never needs its own prior success as qualification.

## Seal, verify and publish

`authorize` reads the real GitHub environment/event and freshly fetched Git refs.
`seal` consumes the frozen candidate plus the accepted external readiness record;
it exclusively creates `release.json` and `checksums.txt`. The final manifest is:

```json
{
  "schema": "batz-release/v0alpha1",
  "version": "v2.0.0-rc.1",
  "source_sha": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
  "candidate": {"name": "candidate.json", "sha256": "8888888888888888888888888888888888888888888888888888888888888888"},
  "readiness": {
    "status_id": 2000,
    "comment_id": 3000,
    "body_sha256": "9999999999999999999999999999999999999999999999999999999999999999",
    "runs": [{"id": 1000, "attempt": 1}],
    "check_ids": [101, 102, 103, 104, 105, 106, 107]
  }
}
```

`verify-set MANIFEST DIST` validates both manifests, exact native archive inventories,
readiness provenance and the complete checksum inventory. DIST contains only the
two archives, `candidate.json`, `release.json`, `checksums.txt`. Checksums cover both
archives and both manifests, excluding themselves; there is no circular hash.

```sh
runtime/.local/venv/bin/python -I -B ci/release.py authorize
runtime/.local/venv/bin/python -I -B ci/release.py seal "$DIST/candidate.json" "$DIST"
runtime/.local/venv/bin/python -I -B ci/release.py verify-set "$DIST/release.json" "$DIST"
# Only under a later explicit publication grant and actual owner tag event:
runtime/.local/venv/bin/python -I -B ci/release.py publish "$DIST/release.json" "$DIST"
```

The final workflow job alone has `contents: write`. The macOS installation job's
`GH_TOKEN` can only read source and Actions artifacts. The final job's digest-pinned
prepared Linux publisher verifies the staged registry digest with real Docker,
including linux/amd64 and OCI labels `org.opencontainers.image.version`,
`org.opencontainers.image.revision`, `org.opencontainers.image.batz-candidate-tree`.
Missing registry configuration or immutable image proof fails before a GitHub
write. This helper never chooses a registry, pushes an image or adds mutable tags.
The selected image must already exist with its verified digest. The preparation
workflow stages GHCR images with repository `packages: write` access and source
labels; the final publisher verifies the digest without adding mutable image tags.

The publisher rechecks tag/source/actors/readiness and local bytes at each write
boundary. It creates a GitHub draft with a manifest-hash marker, uploads missing
identical assets, redownloads and compares the full set, then makes it public.
Attempts serialize per version without cancellation. A matching interrupted draft
can resume; any conflicting identity/bytes fail without deletion or overwrite.
An existing public set reports `already-complete` only after full validation and
redownload. Other versions are untouched; completed older releases stay available.
A failed phase leaves a draft incomplete. Do not repair conflicts by moving a tag
or overwriting a completed asset; new artifacts require a new version and proof.
If an interrupted transfer leaves a non-uploaded/conflicting asset, or reposted
readiness changes the sealed manifest, the automatic retry refuses safely. Under a
separate owner cleanup grant, remove only the affected unpublished draft/asset and
recreate the draft from the identical frozen bytes and accepted proof. Re-seal when
the readiness reference changes; a recreated candidate needs complete qualification.
Never delete/move the tag or overwrite a completed release for recovery. No cleanup
write is authorized by this implementation task.


Focused local evidence uses real temporary Git refs and real small archives with
fake GitHub/registry boundaries. It does not qualify hosted permissions, Linux
confinement, real registry access or a production publication. The release helper
makes no status/comment writes and does not publish through a package manager.

The REST adapter follows GitHub's documented [workflow attempt](https://docs.github.com/en/rest/actions/workflow-runs#get-a-workflow-run-attempt), [attempt jobs](https://docs.github.com/en/rest/actions/workflow-jobs#list-jobs-for-a-workflow-run-attempt) and [release](https://docs.github.com/en/rest/releases/releases) endpoints. Attempt membership comes from the attempt-specific jobs endpoint; jobs need not repeat an attempt number.
