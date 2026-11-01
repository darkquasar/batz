# Run status and model retry

`run` acquires the selected local source and publishes checked results. `retry`
uses committed parsed evidence for the same binding; its source path is an
identity selector and can refer to a file that no longer exists.

```sh
runtime/.local/batz status --workspace /tmp/batz-example
runtime/.local/batz status --workspace /tmp/batz-example --run-id RUN_ID
runtime/.local/batz retry pipelines/aws-cloudtrail/pipeline.yaml \
  --project . --workspace /tmp/batz-example --route local_raw \
  --binding-id demo --input path=/original/source.json --run-id FAILED_RUN_ID
```

Status reads a consistent SQLite snapshot without taking the mutation lease.
Its command exits zero when the read succeeds, including when `run.exit` is
nonzero. It reports the recorded run, exact parsed files, current normalized
revision and retry eligibility. Without `--run-id`, it selects the latest run.

Retry bypasses collection and collector binary verification. It verifies the
retained evidence, then runs current SQLMesh content against that exact committed
membership. The optional run ID prevents substituting a newer parsed revision;
a superseded selection is refused. Without it, retry selects the binding's current
parsed inventory. Already published identical membership and content reuse the
existing checked revision. Changed model content is evaluated again.

```text
+------------------+  => model failure  +-------------------+
| Committed B      | =================> | A stays published |
+------------------+                    +-------------------+
         |
         | => retry with retained files
         v
+------------------+  => checks + commit  +-------------------+
| SQLMesh build B  | ===================> | B becomes current |
+------------------+                      +-------------------+
```

Durable parse/mapping rejects make processing quality `partial`; unsupported
mappings alone remain `complete` with separate coverage counts. Default exit is
zero after publication. `--strict` returns 6 for a published partial result and
keeps its revision and rejects. A model failure keeps the prior revision current.
See [recovery and backup](../operations/recovery.md).
