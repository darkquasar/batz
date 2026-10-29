# Batz

Batz is a local-first DFIR log parsing, normalization and analysis toolkit.
Its canonical CLI name is `batz`.

This repository is reserved for the application, native collector/SQL content and
application documentation. The existing research, specifications and review
records live in the peer [batz-specs repository](../batz-specs/), beginning with
the [pipeline specification](../batz-specs/docs/specs/01-dfir-log-pipeline/pipeline-spec.md).

The current local CloudTrail tracer runs a fixture through Bento, SQLMesh and
DuckDB. Offline explain shows declarations, stages, native structures and sample
values, with saved binding/case context and configuration origins. Native preview
is qualified on macOS ARM64; other environments report unavailable.

Start with [local setup](docs/contributing/local-setup.md), the
[first local run](docs/usage/first-local-run.md),
[selection](docs/usage/selection.md), and [explain](docs/usage/explain.md).
Live source acquisition and broader distribution support remain later slices.

The repository was renamed from `waterelemental` on 2026-10-27.
