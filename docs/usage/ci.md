# Pull-request checks

The PR workflow uses standard GitHub-hosted `macos-15` ARM64 runners. Each
selected job gets a fresh VM, checkout, toolchain and writable Go build cache.
The macOS exception allows dependency preparation before tests; Linux's future
prepared-image requirement is unchanged.

Preparation downloads the Go and standalone Python archives named in
`runtime/dependencies.lock.json`, verifies their SHA-256 hashes, installs the
hash-locked Python wheels and downloads/verifies the Go modules. Caches accelerate
preparation; a cache miss performs the same verified setup. Corrupt archives fail
visibly. Tests use `GOPROXY=off`, `GOSUMDB=off` and `GOTOOLCHAIN=local`.

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
Selection uses merge-base and head manifest ownership. Each test lane builds its
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
