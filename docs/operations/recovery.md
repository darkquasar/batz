# Local recovery

Use [status and retry](../usage/runs.md) after an interrupted run. With committed
parsed evidence, retry needs neither the original source file nor a collector
binary. A failed or interrupted model run keeps the previous normalized revision
queryable. Missing or changed retained evidence produces an integrity failure.

Batz obtains the existing workspace lease before recovery. A surviving child
retains that lease after its parent dies; wait for child work to stop before
retrying. Do not delete `owner.lock` to bypass a busy workspace.

```text
+----------------+  => durable placement  +------------------+
| Valid receipt  | =====================> | Immutable files  |
+----------------+                        +------------------+
                                                   |
                                          => SQLite commit
                                                   v
                                          +------------------+
                                          | Visible inventory|
                                          +------------------+
```

Recovery reuses completed collection only when the coordinator recorded clean
child completion and the receipt, context, files and expected predecessor still
verify. A receipt alone does not prove clean exit. Before completion, an ordinary
`run` can reacquire safely; model-only retry cannot invent a missing parsed batch.
After placement but before commit, files remain invisible. After commit, recovery
reloads exact membership, reconstructs report exports and resumes pending model
work. Duplicate delivery commits once. Unreferenced outputs remain diagnostic
evidence; recovery preserves referenced files and does not run broad cleanup.

For a same-root backup, stop mutation, wait for every child to settle and close
workspace state connections. Copy the entire `.batz` directory as one stopped
snapshot, including SQLite state, any WAL sidecars, parsed/source/reject evidence,
normalized catalogs and SQLMesh state under `models/`. Copying a live SQLite main
file alone is insufficient. Restore only into a stopped workspace at the same
absolute root, then check status and query a published catalog before retrying.
Relocation and cross-platform mutable-state continuation remain later slices.

Coordinator metadata upgrades from Slice 2 preserve committed evidence; unknown
versions are refused. SQLMesh continues to manage its own model lifecycle and
state migrations.
