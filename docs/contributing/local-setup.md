# Local development tools

The first setup used native macOS ARM64 and project-local tools under
`runtime/.local/`. No global toolchain or Python packages were replaced. Candidate
pins and official archive digests are in `runtime/dependencies.lock.json`.
Python dependencies are declared in `runtime/pyproject.toml`. The full Python wheel
lock is `runtime/requirements.lock`; document/schema checks have a separate
`runtime/check-requirements.lock`. Go uses `go.mod/go.sum`.
Passing document checks does not qualify a host or runtime limit.

For an existing verified installation, build and check from the app root:

```sh
export GOTOOLCHAIN=local
export GOMODCACHE="$PWD/runtime/.local/gomod"
export GOCACHE="$PWD/runtime/.local/gocache"
export GOPATH="$PWD/runtime/.local/gopath"
runtime/.local/go/bin/go build -o runtime/.local/batz ./cmd/batz
runtime/.local/go/bin/go test ./... -count=1
runtime/.local/go/bin/go vet ./...
```

The required local Go is 1.27.3 (Bento requires at least 1.27.1). Python is the
3.12.12 standalone archive identified by its exact release and checksum in the
lock. The observed installation used uv 0.8.12, with these dependency commands:

```sh
uv --no-config venv --python runtime/.local/python/bin/python3.12 runtime/.local/venv
uv --no-config pip sync --python runtime/.local/venv/bin/python \
  --require-hashes --only-binary :all: \
  --cache-dir runtime/.local/uv-cache \
  runtime/requirements.lock runtime/check-requirements.lock
```

This uses uv's pip interface with hash-checked requirements locks. A `uv.lock`
workflow (`uv sync` / `uv run`) would be a separate lockfile migration; the
commands here use the existing locks and explicitly selected environment. The
scripts are not an installable Python distribution (`tool.uv.package = false`).
Invoke the explicit interpreter above for scripts and checks.
`runtime/.gitignore` excludes
`.local/`, including downloaded tools, the venv, build outputs and caches; the
root `.gitignore` also excludes Python bytecode, common Python tool caches and
`.venv/` directories. Dependency declarations and lockfiles remain in Git.

To deliberately refresh the locks from the manifest, from the app root:

```sh
uv --no-config pip compile runtime/pyproject.toml \
  --python runtime/.local/python/bin/python3.12 --generate-hashes \
  --output-file runtime/requirements.lock --cache-dir runtime/.local/uv-cache
uv --no-config pip compile --group runtime/pyproject.toml:checks \
  --python runtime/.local/python/bin/python3.12 --generate-hashes \
  --output-file runtime/check-requirements.lock --cache-dir runtime/.local/uv-cache
```

Review dependency and wheel-hash changes before installing. After an intentional
lockfile change, update its digest in `runtime/dependencies.lock.json`; do not use
that step to accept an unexplained checksum mismatch. A lock for one qualified
host does not establish support for other platforms.

On a new machine, select its exact archive from the lock, download to
`runtime/.local/downloads`, verify SHA-256 before unpacking or execution, and stop
on any mismatch. Never update an expected archive/wheel digest to accommodate a
bad download. The downloaded Go and Python archives and original setup logs are
retained in the ignored local directory on this development machine.

Build the owned collector with the same Go environment:

```sh
runtime/.local/go/bin/go build -o runtime/.local/batz-collector ./cmd/batz-collector
```

A deliberately rebuilt collector needs a new `installed_binaries` receipt in
`runtime/dependencies.lock.json`, bound to its reviewed source and selected pins.
That receipt contains the actual SHA-256 of the collector and Python interpreter;
execution checks them before creating a workspace. Do not regenerate this receipt
to bypass an unexplained integrity failure. Setup/bootstrap automation and Linux
native/container qualification are later S9 deliveries, not completed here.

The S1 tests run real Bento, SQLMesh and DuckDB in isolated temporary workspaces.
They are not mocked runtime support claims. Native checks and intended failing
negative controls are retained in `docs/contributing/evidence/s1/`. All commands
use normal permission checks; no agent permission bypass is permitted.

For the composed S2 checks, select a Staticcheck build compatible with the exact
pinned compiler and run:

```sh
BATZ_STATICCHECK=/absolute/path/to/staticcheck \
  runtime/.local/venv/bin/python -I docs/contributing/verify-s2.py
```

This builds the CLI, runs the full tests and supervisor race checks, verifies
contracts and dependency integrity, and validates all three actual CLI report
levels. It records exact inputs, commands, exits and logs under
`docs/contributing/evidence/s2/completion/`. It installs nothing. The retained
checker receipt describes the isolated sandbox build used for S2 verification;
application dependency pins and global tools are unchanged.

For a short development loop, run configuration tests and the static CLI cases:

```sh
runtime/.local/go/bin/go test ./internal/config -count=1
runtime/.local/go/bin/go test ./internal/app -count=1 \
  -run 'Test(BareExplain|ExplainStages|ExplainPreservesTimestamp|ExplainHistorical|RunAggregates|Preflight|CallerRelativeWorkspace|UnresolvedWorkspace|UnsupportedDeclaredOutput|PreviewRuntimeChecksum|PreviewRejects|InvalidReviewedExpectations)'
```

These checks avoid SQLMesh runs. Expectation validation runs eight invented-data
cases in one Python process. Native report assertions share the two fresh runs
needed for repeatability. Native publication, failure-isolation and repeatability
tests retain independent state. The full suite remains the completion gate;
fixture size is already one record for ordinary preview tests. Larger history and
multi-binding performance belong to their later slices and need separate scaling
evidence; current timings do not establish the whole application's asymptotic cost.
