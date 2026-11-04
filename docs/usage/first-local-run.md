# First local CloudTrail run

This is the narrow S1 development path: one uncompressed CloudTrail `Records`
JSON file, with a compact S3 GetObject/IAMUser mapping. Other valid events remain
in parsed/source-staging evidence with unsupported mapping coverage. This is not
full CloudTrail support. RAM is monitored without a Batz cutoff; each run reports average and peak usage
for collection/model processes. Scratch uses a sampled 1 GiB stop threshold, so
brief overshoot is possible. See the [owner decision](../contracts/s1-macos-budget-decision.md).

From the app checkout, after the project-local tools have been prepared:

```sh
runtime/.local/batz explain pipelines/aws-cloudtrail/pipeline.yaml
runtime/.local/batz run pipelines/aws-cloudtrail/pipeline.yaml \
  --project . --workspace /tmp/batz-example --route local_raw \
  --binding-id demo --input path=repo://pipelines/aws-cloudtrail/fixtures/api-read.json
```

`repo://pipelines/aws-cloudtrail/fixtures/...` resolves inside this checkout. The bundle contains
synthetic test data; another explicit regular file path can be selected. Execution
requires an explicit workspace. Reusing `demo` retains its prior parsed membership;
Unchanged committed files are skipped; new/changed file contents extend the
retained membership. See [reruns and replay](runs.md).

```text
+-------------+  native Bento  +----------------+  native SQLMesh  +----------------+
| Source JSON | =============> | Parsed Parquet | ===============> | Reader catalog |
+-------------+                +----------------+                  +----------------+
```

The command emits a JSON RunReport with the exact catalog path and revision.
Use that path in ordinary DuckDB SQL:

```sql
ATTACH '/absolute/path/from/report/catalog' AS revision_a (READ_ONLY);
SELECT event.action, "user".id, "@timestamp", batz.origins
FROM revision_a.demo.api_activity;
SELECT * FROM revision_a.demo.source_staging;
SELECT * FROM revision_a.demo.parsed;
SELECT * FROM revision_a.batz_inventory.membership;
```

Each publication has a separate immutable catalog and literal Parquet membership.
A later run returns another catalog; the prior catalog remains queryable. The
physical catalog currently lives alongside its normalized revision; use the
reported path instead of deriving it from an example directory layout. Catalog
relocation/setup is later work; the inventory retains artifact-relative data paths.

Full source payloads and origins remain available. Source timestamps keep supplied
precision, original text and offsets; synthetic tests distinguish nanosecond
storage from the second-granular source fixture. Undated records have null time.

Bare explain requires no workspace and executes no collector or SQLMesh project.
See [explain](explain.md) for declared stages and offline sample previews. Saved bindings and configuration layering are available; see
[status and model retry](runs.md). Remote routes and portable reader setup remain
later plan slices. Do not mistake their absence for successful empty output.

Default run exit is zero for checked publication, including record-level partial;
`--strict` returns 6 for published partial and keeps that revision. Invalid input,
missing capability, integrity failure and busy ownership have distinct nonzero
exits. A native/model failure cannot replace the prior normalized pointer.

Start with a fresh workspace for the shared root catalog; old development
workspaces and former separate native projects are not supported. Follow
`pipelines/aws-cloudtrail/pipeline.yaml` to its `input/` and `fixtures/`, then
`models/aws-cloudtrail/` and `tests/aws-cloudtrail/` for native SQL and expectations.
