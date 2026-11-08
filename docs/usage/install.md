# Install a native candidate

Batz native archives contain the CLI, its repo-built collector, native SQL/configs,
CloudTrail examples, schemas, SQLMesh bridge and minimum common profile, dependency
declarations, setup metadata and notices. The installation is independent of Git.
The native targets are macOS arm64 and Linux amd64; qualification evidence must name
the actual target. Linux/container support remains unqualified until its required
confinement and installed checks pass. Local-file CloudTrail is the shipped source;
live acquisition, R2, GCP and Workspace routes remain deferred.

Download the archive and its checksum from the same verified release set. Use the
release's published checksum, replacing the placeholders below with that release's
version and platform. No first production version is selected by this guide.

```sh
test -f batz-VERSION-darwin-arm64.tar.gz
shasum -a 256 -c checksums.txt --ignore-missing
mkdir -p "$HOME/Batz installations"
tar -xzf batz-VERSION-darwin-arm64.tar.gz -C "$HOME/Batz installations"
export BATZ_INSTALL="$HOME/Batz installations/batz-VERSION-darwin-arm64"
"$BATZ_INSTALL/bin/batz" version --format json
cd "$BATZ_INSTALL"
mise trust
mise install
```

Use `linux-amd64` for the Linux archive. Compare the hash before extraction.
`version` requires no Python, workspace or setup. Normal mise trust is required;
the uv tool's always-run postinstall prepares the owned environment even if all
tools already exist. The configuration hook verifies that preparation. Repeat `mise install` to repair a missing environment. Setup consumes locked
dependencies and never uses an ambient Python environment. Missing/corrupt tools,
locks, receipts or release content refuse dependent execution. Locked wheel hashes
are enforced during setup. Runtime admission checks the interpreter, collector,
environment ownership and declarations; it does not rehash installed Python packages
or the complete copied standard library on every invocation.
If the environment was modified, rerun `mise install` to reconstruct it.

```text
  +------------------+  resolves symlinks  +---------------------+
  | bin/batz         | =================> | Installation root   |
  +------------------+                    +---------------------+
                                                   | => verifies inventory
                                                   v
                                         +---------------------+
                                         | Content + collector |
                                         +---------------------+
                                                   | => uses isolated Python
                                                   v
                                         +---------------------+
                                         | Explicit workspace  |
                                         +---------------------+
```

Run from a separate directory outside Git, with spaces in both installation and
workspace paths. All mutable investigation state belongs in the explicit workspace.

```sh
mkdir -p "$HOME/Batz example caller"
cd "$HOME/Batz example caller"
export BATZ_WORKSPACE="$HOME/Batz workspaces/example"
"$BATZ_INSTALL/bin/batz" explain "$BATZ_INSTALL/pipelines/aws-cloudtrail/pipeline.yaml" --level 2
"$BATZ_INSTALL/bin/batz" run "$BATZ_INSTALL/pipelines/aws-cloudtrail/pipeline.yaml" \
  --workspace "$BATZ_WORKSPACE" --binding-id example --route local_raw \
  --input "path=$BATZ_INSTALL/pipelines/aws-cloudtrail/fixtures/wiring.json"
"$BATZ_INSTALL/bin/batz" explain "$BATZ_INSTALL/pipelines/aws-cloudtrail/pipeline.yaml" \
  --level 3 --example wiring --format json --strict
"$BATZ_INSTALL/bin/batz" status --workspace "$BATZ_WORKSPACE"
```

Level 1/2 explain stays static and does not provision a runtime. Level 3 executes an
offline sample in private scratch. Ordinary execution disables Python bytecode writes
and copies the native project into owned scratch/workspace before framework loading.
The installed native content remains unchanged. A tampered or missing native file,
mixed collector/CLI, or foreign interpreter fails before acquisition/publication.

After a model failure with committed parsed evidence, use the same `retry` command
and selection to rebuild from that evidence:

```sh
"$BATZ_INSTALL/bin/batz" retry "$BATZ_INSTALL/pipelines/aws-cloudtrail/pipeline.yaml" \
  --workspace "$BATZ_WORKSPACE" --binding-id example --route local_raw \
  --input "path=$BATZ_INSTALL/pipelines/aws-cloudtrail/fixtures/wiring.json"
```

Use the exact `catalog` path from the run report with the installation's reader
runtime, even while the caller remains outside Git:

```sh
"$BATZ_INSTALL/runtime/.local/venv/bin/python" -I -B - /absolute/catalog/from/report <<'PYREADER'
import duckdb, sys
connection = duckdb.connect(sys.argv[1], read_only=True)
print(connection.execute('SELECT event.action, "user".id, "@timestamp", batz.origins FROM example.api_activity').fetchall())
print(connection.execute('SELECT * FROM batz_inventory.membership').fetchall())
PYREADER
```

See [first local run](first-local-run.md) for additional DuckDB/Parquet queries,
[reruns and recovery](runs.md) for retry preconditions and
[explain](explain.md) for retained sample evidence. Reader catalogs are generated
artifacts; do not open mutable SQLMesh state as published evidence.

`--project /absolute/content-root` selects that root explicitly. Otherwise a
containing checkout takes precedence; an unrelated Git checkout does not discover
peer or installed content. When working inside an unrelated checkout, use
`--project "$BATZ_INSTALL"` for installed execution; otherwise missing release
content fails integrity admission. Static explain of explicitly selected checkout
content still reports declarations and unavailable/integrity diagnostics; `--strict`
refuses incomplete reports. Outside a checkout, the resolved `bin/batz` executable
selects its own installation, including through an active symlink. Runtime tools
never fall back to PATH. Release inventories and installation receipts describe
different things: the inventory binds shipped bytes, while the receipt records
setup observations checked against immutable dependency declarations.

Prepare upgrades side by side in a new directory, verify checksums, run setup and
complete a local example before selecting its `bin` directory or updating an active
symlink. If preparation fails, keep the previous installation selected. Retain its
workspace and reports; select its original binary to return to it. Do not overwrite
an existing release directory. Alpha report/content contracts can break; use a fresh
workspace for an incompatible version. Cross-version mutable-state continuation and
future S8 relocation are outside the first release qualification.

For maintainers, `python3 packaging/build.py archive --help` documents archive
creation. Official builds require clean source at the exact supplied full revision.
Local `--development` candidates retain dirty/source-tree identity and cannot qualify
as official releases. `candidate --help` assembles both native archive records with
a real immutable Docker digest; it does not publish or invent a registry identity.

The Docker distribution includes Batz, its collector, Python environment and native
pipeline content. Your input and workspace folders stay outside the image. Select
the `ghcr.io/darkquasar/batz@sha256:...` runtime reference recorded in the release
manifest as `BATZ_IMAGE`. For a Linux amd64 host, run a local CloudTrail file with:

```sh
mkdir -p "$PWD/workspace"
docker run --rm --user "$(id -u):$(id -g)" \
  --mount "type=bind,src=$PWD/evidence,dst=/evidence,readonly" \
  --mount "type=bind,src=$PWD/workspace,dst=/workspace" \
  "$BATZ_IMAGE" run \
  "/opt/Batz installations/candidate/pipelines/aws-cloudtrail/pipeline.yaml" \
  --workspace /workspace --binding-id example --route local_raw \
  --input path=/evidence/cloudtrail.json
```

Using your user/group IDs keeps workspace results readable by your host user.
The image uses its prepared environment; ordinary commands do not install tools.
Results and run state remain in the mounted workspace after the container exits.
