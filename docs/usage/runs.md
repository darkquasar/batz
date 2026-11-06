# Reruns, replay, status and model retry

`run` selects one local file or a flat directory and publishes checked results.
For CloudTrail, a directory selects regular `.json` files; the internal JSONL
probe selects `.jsonl`. Subdirectories and other extensions are outside selection.
Selected symlinks fail. Selection is sorted, frozen and SHA-256 checked before
collection. New directory arrivals wait for the next run. The initial limits are
128 selected files, 16 MiB of aggregate selected bytes and 10,000 newly acquired
records per invocation. Collection remains serial; the existing child time and
scratch limits still apply.

Ordinary reruns skip path/content identities already committed for this binding,
acquire new or changed contents, and preserve all earlier observations. A filename
or timestamp alone never proves unchanged. Changed bytes between selection and
collection fail acquisition. An entirely unchanged run reuses its checked
publication when execution content is unchanged; changed SQL rebuilds retained
evidence. An initially empty directory has no explicit typed input to publish:
use a `Records: []` JSON document for a checked empty acquisition.

Use `--replay` with `run` to acquire every selected file again. Replay adds fresh
source observations; it differs from model-only `retry`.

```sh
bin/batz run pipelines/aws-cloudtrail/pipeline.yaml \
  --project . --workspace /tmp/batz-example --route local_raw \
  --binding-id demo --input path=/evidence/cloudtrail
# Same selection, deliberately acquired again:
bin/batz run pipelines/aws-cloudtrail/pipeline.yaml \
  --project . --workspace /tmp/batz-example --route local_raw \
  --binding-id demo --input path=/evidence/cloudtrail --replay
```

The report's `acquisition` lists selected `new`, `changed`, `skipped`, `replayed`
and actually committed `acquired` inputs separately. Files commit individually
through the existing durable receipt boundary. If a later file fails, no new
normalized revision publishes; earlier file commits remain available to retry.
Recovery resumes the interrupted frozen selection before considering later
arrivals. Changing a binding's source location, pipeline identity/version or
framing requires a new binding.

`reconciliation` counts mapped observations, distinct variants, identified logical
events, conflicting identified groups and unidentified observations. These counts
are separate from parse/mapping quality and input acquisition accounting.

See [CloudTrail identity and query examples](../sources/cloudtrail.md). `retry`
uses committed parsed evidence for the same binding; its source path is an
identity selector and can refer to a file that no longer exists.

```sh
bin/batz status --workspace /tmp/batz-example
bin/batz status --workspace /tmp/batz-example --run-id RUN_ID
bin/batz retry pipelines/aws-cloudtrail/pipeline.yaml \
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
