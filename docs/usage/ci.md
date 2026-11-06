# Pull-request checks

The PR workflow uses standard GitHub-hosted `macos-15` ARM64 runners. Each
job gets a fresh VM. Its preparation action bootstraps the exact mise version in
`runtime/dependencies.lock.json`, with official binary SHA-256 verification, then
runs normal `mise trust` and `mise install`. The macOS exception permits online
preparation before offline tests. Linux still requires a digest-pinned prepared
image; these declarations alone do not qualify native Linux or Docker support.

Mise's HTTP backend preserves the declared standalone Python 3.12.12 and Go 1.27.3
archive bytes. Uv 0.8.12 installs the exact root `uv.lock`; its 63 runtime/check
package versions and allowed artifact hashes match the retained requirements
locks. Setup never resolves a stale lock, builds Python packages, invokes pip or
uses a system/active Python environment. The uv tool's `when = "always"`
postinstall waits for Python/Go and runs `ci/setup.py`; a failure fails installation.
The config-level postinstall invokes the same entry point with `--check`.
This combination also repairs a deleted environment when tools are already present.
See mise's [tool hooks](https://mise.jdx.dev/dev-tools/#tool-postinstall-commands)
and [config hooks](https://mise.jdx.dev/hooks.html#install-hooks).

Every checkout/installation owns `runtime/.local/python` and a uv-created
`runtime/.local/venv`. Setup reconstructs this environment from locked wheels on
each invocation, removing undeclared packages and site hooks. It clears inherited
Python, uv, pip, Conda and active-venv settings and selects an absolute owned uv
cache/environment. It refuses runtime-directory symlinks and system site packages.
`BATZ_TOOLCHAIN_ROOT` cannot select another checkout's environment. Source checkouts
also download/verify their pinned Go modules and build their own collector;
installed archives keep the collector admitted by their immutable release inventory.
After a reviewed Python or Go pin change, prepare a fresh source checkout with normal
trust and `mise install`. An existing checkout's owned interpreter/toolchain may
fail integrity admission against the new pins; setup does not silently replace
those bases. Preserve the old checkout and investigation state. Installed release
upgrades use separate directories as described in [installation](install.md).


`runtime/.local/installation.json` is an observed `batz-installation/v0alpha1`
receipt with target, exact runtime-lock SHA-256 and Python path/digest. Expected
Python and uv digests come from verified declared archives in the immutable lock.
A development checkout additionally records its collector digest and source digest.
The source digest hashes sorted POSIX paths for `go.mod`, `go.sum` and all `.go`
files under `collector/`, `cmd/batz-collector/` and `internal/buildinfo/`: path,
NUL, file bytes, NUL for each file; symlink files are refused. It is a development
receipt, not published-release authority. `ci/test` and `ci/select` verify the
lock-bound receipt offline before running; they never install or sync.

Download and Go caches live under absolute runner-owned paths and accelerate
preparation. Corrupt cached archives fail visibly; remove the offending cache entry
before retrying. Tests set `GOPROXY=off`, `GOSUMDB=off` and `GOTOOLCHAIN=local`.
Explicit `-I -B` Python launches ignore ambient Python configuration and keep
bytecode out of immutable content. Setup, cold compilation and queue time remain
separate from test durations.

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
an invalid head fails. Unknown impact cannot become docs-only success. Preparation
builds the collector from the checked-out revision and records its source/binary identity. App and adapter/combined-model lanes verify and reuse that
prepared collector; they do not rebuild it or create Python environments. Runtime
work remains private to each test. Native-only model jobs skip Go build-cache
transfer; source setup still builds its matching collector. Go test-result caching is disabled.

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

For local preparation with the pinned mise executable on `PATH`, run from the
checkout or extracted installation root:

```sh
mise trust
mise install
ci/test fast
```

Normal mise trust checks remain enabled. Repeating `mise install` recreates only
that root's environment. To exercise missing-environment repair, remove only that
root's `runtime/.local/venv` and repeat `mise install`. A failed setup leaves no new
valid receipt; offline checks refuse missing, stale or wrong interpreter receipts.
Prepare an upgrade in a separate installation root before selecting it; the prior
installation and workspaces remain available if the new setup fails.

The hosted bootstrap is `python3 -I -B ci/prepare-macos.py`; system Python uses
only its standard library to verify/download mise. It writes no system packages.
`BATZ_DOWNLOAD_CACHE`, `GOMODCACHE`, `GOCACHE` and `GOPATH` may name task-owned caches.
To isolate mise itself, set absolute `MISE_DATA_DIR`, `MISE_CACHE_DIR`,
`MISE_CONFIG_DIR` and `MISE_STATE_DIR` before both trust and install. Every root
still receives its own copied Python, uv environment and receipt.

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
