# Release support and qualification

The first release is scoped to saved local-file CloudTrail input. CloudTrail S3/R2
acquisition and credentials, GCP Audit exports and Workspace exports remain deferred.
S8 remains planned after the first release: multi-binding scheduling, concurrent
reader qualification and portable reader bundles are future capabilities. Existing
single-binding isolation, evidence integrity, recovery and safe publication remain
required.

The supported installation targets remain native macOS arm64, native Linux amd64
and Linux amd64 Docker. A built archive or a cross-compiled binary establishes no
native qualification. A missing host, confinement capability, image or access
leaves that target unqualified; it does not reduce the promised target set.

```text
+--------------------+ exact candidate +----------------------+ qualified set +------------------+
| Source + native SQL| ===============>| Existing CI contracts| =============>| Release readiness|
+--------------------+                 +----------------------+               +------------------+
```

A release requires the complete shipped catalog through the existing commands:

```sh
ci/test fast
ci/test app
ci/test models --mode native
ci/test models --mode adapters
```

`ci/test all-local` composes the local lanes. Required checks must actually run
and pass; omitted, skipped, cancelled, empty and missing required work cannot
qualify a release. Shared application proof uses invented internal content. Actual
pipeline native/adapter contracts prove source meaning without repeating the
application lifecycle matrix for each pipeline.

Installation qualification additionally exercises the documented native candidate
outside Git, including normal trust, fresh/repeated setup, missing-environment
repair, version identity, explicit workspace, run/retry, explain, bounded offline
sample, ordinary reader and separate-root upgrade preservation. Reuse compatible
assertions from that walkthrough; do not create a platform-pair or per-pipeline
application test matrix.

GitHub-hosted macOS arm64 may prepare verified pinned dependencies in each fresh
VM before offline tests. Linux CI requires an already prepared, digest-pinned tool
and fixture image. Ordinary tests never install/sync dependencies or fetch baked
fixtures. A prepared tool image, the tested app/content revision and the distributed
runtime image have separate identities; a stale baked app cannot qualify a new SHA.

Record actual host architecture, source/input/candidate digests, command exits,
required checks and target outcomes. Report setup, cold compilation and hosted
queue time separately when measured. Keep verification inexpensive and simplify
redundant work; there is no numerical performance pass/fail gate. Finite execution,
process and resource safety limits remain required.

## Local S9 readiness limits

The authoring host is native macOS arm64. The Docker CLI is installed but its daemon
socket is unavailable; a native Linux amd64 host has not been provided. Current
Linux offline confinement is unavailable and fails closed. Linux and Docker native
execution, installation and confinement qualification therefore remain open.
Cross-builds or recipe inspection cannot replace those results.

The first production version/channel and Docker registry/namespace/access remain
owner decisions. Local setup, archive generation and failure-path tests do not
choose them. Production tags, GitHub releases, image publication and package
publication require a separate grant. No package-manager automation is required.

## Docker preparation recipes

The two recipes take a required `PREPARED_IMAGE` pinned as `name@sha256:digest`;
there is no default registry or base-image choice. The supplied Linux amd64 base
must already contain the verified pinned mise bootstrap, CA certificates, curl,
tar and procps (plus Git for the source qualification recipe). Build preparation may install the declared mise tools and uv
packages. An ordinary image invocation only executes the selected CLI.

`packaging/Dockerfile.ci` prepares tools and local fixtures; record the resulting
image digest and fixture source. Tests must explicitly mount/select the source
revision under qualification and use the prepared tools offline. The current
Linux admission/confinement gap must be resolved and natively qualified before
this image can supply a passing release lane.

`packaging/Dockerfile.runtime` consumes a verified native Linux archive from
`dist/`, its SHA-256, version and full source SHA. It extracts that complete
installation at `/opt/Batz installations/candidate`, prepares its owned environment and checks CLI identity.
The Docker context filters exclude checkout caches, credentials and local state.
Supply an explicit workspace volume at runtime; application data belongs outside
the installation. Build/run and the resulting immutable registry digest still need
actual Linux/Docker access and owner publishing configuration.

The workflow's prepared Linux source/tool root is `/opt/batz`; candidate native
archives/manifests are staged at `/opt/release-dist`. Installed probes use the
separate `/opt/Batz installations/candidate` root. Qualification validates the
actual source SHA and candidate identities at these locations. An image containing
older source or another candidate must fail; the prepared-image digest alone does
not qualify the current release. No such exact-source image has been supplied or
qualified in this local session.

Build the CI recipe from a clean standalone clone at the intended exact SHA;
linked-worktree `.git` pointer files are insufficient. The context keeps local Git
identity objects/refs/index/HEAD while excluding config, hooks, logs and worktrees.
Provide the frozen candidate set under `dist/`, `LINUX_ARCHIVE` and its reviewed
`LINUX_ARCHIVE_SHA256`. Image preparation verifies clean source identity, prepares
the source tools and independently sets up the extracted Linux installation. The
ordinary workflow then verifies the candidate set and runs offline. The runtime
recipe takes only its native archive in `dist/`; its context excludes Git entirely.
