# Batz

Batz is a local-first DFIR log parsing, normalization and analysis toolkit.
Its canonical CLI name is `batz`.

This repository is reserved for the application, native collector/SQL content and
application documentation.

The current local CloudTrail tracer runs a fixture through Bento, SQLMesh and
DuckDB. Offline explain shows declarations, stages, native structures and sample
values, with saved binding/case context and configuration origins. Native preview
is qualified on macOS ARM64; other environments report unavailable.

## Pipelines, bindings, workspaces and cases

A **binding** is one configured use of a reusable pipeline: the pipeline version,
route, input and settings to use, with an ID for tracking its imports and reruns.
For example, two CloudTrail bindings can read evidence from different accounts
using the same parsing and SQL logic:

```text
+---------------------+  configure  +----------------------------+
| CloudTrail pipeline | =========> | account-a: /evidence/a/    |
+---------------------+            +----------------------------+
          |
          | => configure
          v
+----------------------------+
| account-b: /evidence/b/    |
+----------------------------+
```

| Term | What it means |
| --- | --- |
| Pipeline | Reusable source definition with native collection configuration and SQL models. |
| Binding | One configured instance of a pipeline. Its ID separates input tracking, state and published results from other bindings within a workspace. |
| Workspace | The explicitly selected location holding evidence, run state, reports and published data. |
| Case | An optional saved group of bindings for an investigation. |

Supply a binding through CLI arguments or save it in a binding file. Reuse its
stable ID in the same workspace to address its existing history; omitting the ID
generates a new binding. A single-binding run needs no case. The current CLI can
select one binding from a case; scheduling several case bindings together remains
planned. See [selection and saved bindings](docs/usage/selection.md) and
[reruns and replay](docs/usage/runs.md).

## Getting started

Start with the [runtime pins](runtime/dependencies.lock.json),
[first local run](docs/usage/first-local-run.md),
[selection](docs/usage/selection.md), and [explain](docs/usage/explain.md).
Live source acquisition and broader distribution support remain later slices.

For new data sources and pipelines, follow the [contributor journey](contributing.md).
See [CI and toolchain preparation](docs/usage/ci.md) for pinned macOS setup.

After the pinned tools have been provisioned, local development and CI use the
same commands:

```sh
ci/test fast
ci/test app
ci/test models aws-cloudtrail --mode native
ci/test models aws-cloudtrail --mode adapters
ci/test all-local
```

Bare `go test ./...` runs the fast contracts. Native lanes require the qualified
macOS ARM64 host and installed tools; requested missing work fails visibly.
App and adapter/combined-model invocations build their collector and dependency
receipt in private scratch. Native-only model tests need no collector build. `BATZ_TOOLCHAIN_ROOT` selects provisioned tools outside the checkout;
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
