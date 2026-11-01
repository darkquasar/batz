# Pull-request checks

The PR workflow uses standard GitHub-hosted `macos-15` ARM64 runners. Each
job gets a fresh VM. Selection and test jobs prepare private toolchains;
test jobs also restore their own writable Go build caches.
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

CI calls `ci/test fast`, `ci/test app`, `ci/test models <pipeline>` and
`ci/test e2e <pipeline>`. Documentation-only changes run whitespace checks.
Selection uses merge-base/head native model provenance and dependency graphs,
plus manifest-owned assets. Removed edges retain former-consumer checks. An old
layout base visibly broadens lower-layer coverage with fixed compatibility journeys;
an invalid head fails. Unknown impact cannot become docs-only success. Each native lane builds its
collector from the checked-out revision in private scratch; caches never replace
that source build. Go test-result caching is disabled.

The final `gate` requires successful selection and rejects failed or cancelled
jobs. Fork PRs remain unsupported and fail the gate. Jobs receive read-only
repository permissions and no private transport credentials. Dependency setup
and cache restoration are separate workflow steps; their durations and queue
wait are not the accepted reference-host test timings. The first build on an
empty cache also includes compilation. GitHub cache scope may cause a cold setup
on a new PR; cache availability is not a correctness prerequisite.

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
`ci/test models A B` also verifies each actual adapter and its staging integration.
CI can separate these responsibilities using the same command's `--mode native`
and `--mode adapters` options. Private state and two native host slots apply.
SQLMesh graph discovery requires the pinned Python dependencies; setup and CI queue
time are reported separately from warm local test measurements. Native graph
regressions run in `app` and are included in `all-local` cost, not hidden in fast.
