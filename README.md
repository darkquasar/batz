# Batz

Batz is a local-first DFIR log parsing, normalization and analysis toolkit.
Its canonical CLI name is `batz`.

This repository is reserved for the application, native collector/SQL content and
application documentation. The existing research, specifications and review
records live under [specs/batz](../specs/batz/) in the shared peer specs repository.
Start with the [pipeline specification](../specs/batz/01-dfir-log-pipeline/pipeline-spec.md)
and [implementation plan](../specs/batz/01-dfir-log-pipeline/pipeline-plan.md).

The current local CloudTrail tracer runs a fixture through Bento, SQLMesh and
DuckDB. Offline explain shows declarations, stages, native structures and sample
values, with saved binding/case context and configuration origins. Native preview
is qualified on macOS ARM64; other environments report unavailable.

Start with the [runtime pins](runtime/dependencies.lock.json),
[setup and qualification scope](../specs/batz/01-dfir-log-pipeline/pipeline-plan.md), and the
[first local run](docs/usage/first-local-run.md),
[selection](docs/usage/selection.md), and [explain](docs/usage/explain.md).
Live source acquisition and broader distribution support remain later slices.

For new data sources and pipelines, follow the [contributor journey](contributing.md).

The repository was renamed from `waterelemental` on 2026-10-27.

After the pinned tools have been provisioned, local development and CI use the
same commands:

```sh
ci/test fast
ci/test app
ci/test models aws-cloudtrail
ci/test e2e aws-cloudtrail
ci/test all-local
```

Bare `go test ./...` runs the fast contracts. Native lanes require the qualified
macOS ARM64 host and installed tools; requested missing work fails visibly.
Each native invocation builds its own collector and dependency receipt in private
scratch. `BATZ_TOOLCHAIN_ROOT` selects provisioned tools outside the checkout;
the local default is `runtime/.local`. Tests do not update installed assets or
the tracked lock. Pipeline model tests use the first declared explain example as
the representative adapter fixture and its declared route/checksum. The selector
reads manifest ownership at merge-base and head; mapping tests remain native
SQLMesh tests, while Go owns shared lifecycle and publication checks.

`all-local` excludes real R2 transport, live-source acquisition, Linux native and
container runs, other platforms and actual CI activation. Those need separate
qualification. The authored PR workflow's `gate` is the intended required check
when runners and branch protection are activated. Fork runs fail that gate;
manual qualification uses `ci/test all-local`.
