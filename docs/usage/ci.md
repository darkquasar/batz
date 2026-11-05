# Pull-request checks

The PR workflow uses standard GitHub-hosted `macos-15` ARM64 runners. Each
job gets a fresh VM. Selection and test jobs prepare private toolchains;
test jobs also restore their own writable Go build caches. The preparation action
writes absolute `$RUNNER_TEMP/batz-toolchain`, `$RUNNER_TEMP/batz-cache/{downloads,gomod,gocache}`
and `$RUNNER_TEMP/batz-gopath` paths to `$GITHUB_ENV` before restoring caches.
Generated content stays outside the checkout; cache patterns contain no `..`.
The macOS exception allows dependency preparation before tests; Linux's future
prepared-image requirement is unchanged.

Preparation downloads the Go and standalone Python archives named in
`runtime/dependencies.lock.json`, verifies their SHA-256 hashes, installs the
hash-locked Python wheels and downloads/verifies the Go modules. Caches accelerate
preparation; a cache miss performs the same verified setup. Corrupt archives fail
visibly; delete the offending entry with `gh cache delete <key>` before retrying.
Tests use `GOPROXY=off`, `GOSUMDB=off` and `GOTOOLCHAIN=local`.

```text
+----------------+  pinned setup  +------------------+  selection  +----------------+
| Fresh macOS VM | =============> | Manifest changes | ==========> | Selected jobs  |
+----------------+                +------------------+             +----------------+
                                                                          | results
                                                                          v
                                                                  +----------------+
                                                                  | Required gate  |
                                                                  +----------------+
```

CI calls `ci/test fast`, `ci/test app` and `ci/test models <pipeline>`
(with `--mode native` or `--mode adapters` for separate jobs). Documentation-only changes run whitespace checks.
Selection uses merge-base/head native model provenance and dependency graphs,
plus manifest-owned assets. There is no production E2E matrix or filename/tag requirement. Removed edges retain former-consumer checks. An old
layout base visibly broadens lower-layer coverage with shared synthetic compatibility proof;
an invalid head fails. Unknown impact cannot become docs-only success. App and adapter/combined-model lanes build their
collector from the checked-out revision in private scratch; native-only model tests
need no collector build and skip the Go build-cache transfer; caches never replace
that source build. Go test-result caching is disabled.

The final `gate` requires successful selection and rejects failed or cancelled
jobs. Fork PRs remain unsupported and fail the gate. Jobs receive read-only
repository permissions and no private transport credentials. Dependency setup
and cache restoration are separate workflow steps; their durations and queue
wait are not the accepted reference-host test timings. The first build on an
empty cache also includes compilation. GitHub cache scope may cause a cold setup
on a new PR; this PR-only workflow enables reuse on subsequent runs of the same
PR, without populating a default-branch cache. Exact build-cache keys are immutable:
later commits may still compile against the first saved cache. Cache availability
is not a correctness prerequisite. Measure dependency preparation, collector build,
Go compilation/test execution, whole-job duration and PR critical path separately.

For local preparation on macOS ARM64, choose a toolchain directory that does not
already exist (the script deliberately refuses to overwrite an installation):

```sh
export BATZ_TOOLCHAIN_ROOT="$PWD/../batz-tools"
export BATZ_DOWNLOAD_CACHE="$PWD/../batz-downloads"
export GOMODCACHE="$PWD/../batz-gomod"
export GOCACHE="$PWD/../batz-gocache"
export GOPATH="$PWD/../batz-gopath"
python3 -I ci/prepare-macos.py
ci/test fast
```

The system Python only bootstraps preparation; tests use the pinned Python.

Selected model suites run as one native batch, with selected and executed test IDs
compared exactly. Shared tests run once; requested empty/omitted suites fail. Owned helper models
are included even when they are outside runtime roots. Selecting the full catalog
(including `all-local`) checks every native test, including unconsumed shared models.
Unconsumed native changes visibly broaden test selection; test-only edits select
native tests without adapter or E2E jobs.
`ci/test models A B` also verifies each actual adapter and its complete native contract: declared
staging/evidence/check/output roots, native audits, production profile/origin/accounting
validation and the representative fixture expectations. Native type/time queries execute
on actual outputs. No app workspace, publication or reader is involved.
CI can separate these responsibilities using the same command's `--mode native`
and `--mode adapters` options. Private state and two native host slots apply.
SQLMesh graph discovery requires the pinned Python dependencies; setup and CI queue
time are reported separately from warm local test measurements. Native graph
regressions run in `app` and are included in `all-local` cost, not hidden in fast.

Shared preview, strictness, file tracking, replay and recovery tests load only
framework-owned synthetic content, including untagged fast application contracts.
The representative `TestAppSharedPipelineJourney` reuses successful run/recovery
executions for run-to-reader assertions. CloudTrail private SQL/manifest/fixture
changes select its own native/adapter/contracts plus fast checks. A pipeline can be
added without an application E2E or core-language test. Internal runtime fixture
content/models select `app`; native YAML test-only edits select native tests only,
including synthetic native tests. Shared SQL uses actual base/head consumers and
selects `app` only when it reaches the synthetic runtime closure. Core/runtime/
collector/helpers and relevant dependencies/global config select shared coverage;
core deletions and removed native edges retain that coverage. Unknown impact
broadens visibly or fails. Docs-only stays cheap.

`ci/test e2e` is retired and fails argument validation. `all-local` runs fast, shared
app and one batched full native/adapter workload, with no per-pipeline E2E loop.
Actual adapter contexts use private writable state under the same two host slots.
The shared Go runner reads JSON events and prints readable assertion output. It
requires executed passing tests and package results for each requested package,
requires the representative shared journey, and rejects any selected skip including
subtests. Native/Python selected omissions and skips fail too. The workflow gate
requires successful selection and rejects failed/cancelled jobs; hosted results
remain unqualified until an authorized matching run provides them.

Adapter jobs share a Go build-cache key within their job/platform/module inputs
because each compiles the same collector target, without a pipeline build tag.
Each VM still owns its writable cache and rebuilds checked-out source. This can
improve later same-PR reuse; it does not remove concurrent cold builds. These
cache changes have no matching hosted save/restore or timing qualification yet.

The current repository synthetic fixture has no dependency on repository
`models/shared` or global native directories; its copy helper keeps it self-contained.
Adding such a consumer also requires adding the consumed shared/global content
to that fixture helper. The existing journey constructs its own invented shared
ancestor, and isolated selector fixtures verify the base/head shared-SQL exception.
