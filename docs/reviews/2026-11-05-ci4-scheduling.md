CI4 scheduling audit is complete. No source changes, experiments, tests, background jobs or delegation were performed.

Reviewed root: `/Users/martinpopolito/GitHub/batz`
Base/HEAD: `b361019c0dd527cb156dd8697cbf153074e0b670` plus captured dirty/untracked bytes.

Input bindings:

- `ci4-inputs.json`: `8ddf128fa0316d08eaa860e363317a7ae0d2e0371983a98f26171f9fd69bdf87`
- `ci4.patch`: `3976eb55420166358f5b0c2161eebb28ab5aca6952a0335dc9822db9c5568bc6`

A read-only SHA-256 comparison of every captured path found **no drift**.

The scheduling structure is sound for the current scope:

```text
+-----------+  selects  +--------------------------+
| Selection | =======> | Independent hosted jobs  |
+-----------+           +--------------------------+

+-----------+  launches  +-------------------------+
| Local app | =========> | Two shared native slots |
+-----------+            +-------------------------+
```

Selected hosted jobs depend on selection, not on one another (`.github/workflows/pr.yml:57`, `:71`, `:85`, `:105`). Each receives a private VM. Local nested jobs share the same two file-lock slots (`ci/test.py:85`, `:124`, `:169`, `:252`), and subprocesses inherit the enclosing lease through `pass_fds`. Native models run in one selected batch; its private scratch, SQLMesh home and state belong to that invocation (`runtime/sqlmesh/test_models.py:173`). There is no remaining production E2E loop.

Findings and dispositions recommended for CI5:

1. **CI4-S1 — Low: native-only models job restores an unused Go build cache.**

   Evidence: `.github/workflows/pr.yml:96` uses the preparation action’s default `build-cache: true`, while its command is exclusively `models --mode native` (`:102`). `ci/test.py:248` explicitly bypasses `prepare_native()` for that mode, so it performs no collector build or Go test compilation.

   Consequence: avoidable cache restore/save transfer and bookkeeping on every selected native-model job.

   Correction: pass `build-cache: 'false'` for this job, as selection already does. Retain pinned Go/module verification and the existing Python setup. The reduction in hosted duration is unmeasured.

2. **CI4-S2 — Low: the shared journey opens separate readers for compatible assertions.**

   Evidence: `internal/app/app_runtime_test.go:403`, `:413` and `:416` each start `query()` against the same recovered catalog. `query()` launches a fresh Python/DuckDB process (`internal/app/native_test.go:26`). The old-catalog count at `app_runtime_test.go:406` duplicates the assertion made by the already-open reader when `finishReader` attaches the recovered catalog (`internal/app/shared_reader_test.go:27`).

   Consequence: three avoidable reader process starts in the shared critical path.

   Correction: group the three recovered-catalog queries into one result, and remove the later standalone old-catalog count while retaining the held-reader comparison. Preserve the earlier old-catalog check immediately after normalization failure (`app_runtime_test.go:346`), which proves a distinct transition.

   This saves starts structurally; no duration saving is established until the affected journey is measured.

3. **CI4-S3 — Evidence limitation, not a code defect: scheduling alone does not establish a passing shared budget.**

   The shared lane occupies one slot with the sequential app package while the other runs safety followed by Python contracts (`ci/test.py:169`).

   Observed CI2 command durations:

   | Work | Seconds |
   | --- | ---: |
   | Collector build/setup | 1.265 |
   | Shared app Go command | 40.407 |
   | Run/scratch safety Go command | 11.115 |
   | Python contracts | 11.803 |
   | Total command work | **64.590** |
   | Shared lane wall time | **41.781** |

   These figures are retained in `ci2-shared.log`. For these observed slot occupancies, ideal redistribution of the three test commands has a 31.663-second lower bound at two slots. This is an observation about the present workload and timings, not a universal performance bound. Rearranging jobs does not demonstrate compliance with the existing 25-second upper tolerance.

   The unchanged host baseline was 44.142 seconds, with 65.262 seconds of logged command work. Neither comparison supports claiming that the required shared budget now passes.

Rejected or deferred alternatives:

- **Additional VMs or shared-test shards:** increase provisioning/build work and redistribute latency without reducing retained work. No current hosted evidence justifies them.
- **Go `t.Parallel()` across native tests:** would bypass the runner’s native slot admission unless additional coordination were introduced.
- **Batching the hosted adapter matrix immediately:** could remove repeated VM preparation and collector builds for broad selections, but current batching rejects different actual parsed schemas (`test_models.py:185`) and refreshes overlapping model scopes with a second plan (`:134`). Those semantics need bounded evidence before changing topology. Retain the matrix in this milestone.
- **Skipping setup for docs using a second path classifier:** risks duplicating ownership logic, including explicitly referenced Markdown fixtures. Selection preparation costs should remain disclosed.
- **Removing discovery on no-op/retry paths:** native scope discovery supplies production identity and ownership validation. Startup cost alone does not justify removing it.
- **Changing adapter cache partitioning merely because E2E disappeared:** unsupported. A shared collector-build cache key can instead be justified by the fact that every adapter builds the same source target; cache correctness remains the caching audit’s responsibility.

Source-audited start counts:

The following are expected successful-path counts derived from current source and retained passing test cases. They are **not an observed complete OS-process inventory**.

| Shared app case | Collector calls | Runtime model Contexts | Discovery Contexts |
| --- | ---: | ---: | ---: |
| Repeated native preview | 2 | 2 | 2 |
| Explicit local preview | 1 | 1 | 1 |
| Strict partial plus model-free retry | 1 | 1 | 2 |
| Empty/growing/change/replay/no-op journey | 8 | 5 | 6 |
| Shared run/recovery/no-op journey | 3 | 4 | 5 |
| Signal cancellation at model launch | 1 | 0–1 interrupted | 1 |
| Busy workspace | 0 | 0 | 1 |
| Missing/foreign model refusal | 0 | 0 | 2 |
| **App subtotal** | **16** | **13 completed + 0–1 interrupted** | **20** |

References: `app_preview_test.go:66`, `:178`, `:228`, `:256`; `app_replay_test.go:17`, `:40`, `:46`, `:52`, `:54`, `:76`; `app_runtime_test.go:33`, `:178`, `:215`, `:291`, `:309`, `:334`, `:392`, `:444`. Each selected acquisition file launches a collector (`internal/run/collection.go:134`). App run/retry preflight creates discovery before coordination (`internal/app/app.go:262`).

Additional shared scope:

- `internal/run/recovery_test.go` implies **10 collector calls and 4 runtime model Contexts**, with no application discovery: its cases call `Execute` directly. This includes one directory recovery model, two initial normalized-checkpoint models and the uncommitted-normalization retry model.
- The shared Python log directly exposes **55 Context creations**: 50 discovery, two runtime, two native-regression and one adapter. The new actual-adapter required-check negative contributes **three collector calls**.
- App assertions launch **11 ordinary `query()` readers plus one held reader**. Preview launches three normal inspection processes and two inspection-failure processes.

Estimated current shared total: **29 collector calls; 92 completed Context creations plus 0–1 interrupted runtime Context; 12 ordinary/held readers and five preview inspectors.** Corresponding baseline estimates are 26 collectors and 85 completed Contexts plus the same cancellation uncertainty; readers/inspectors are unchanged.

Limitations: these counts exclude sandbox qualification processes, Go helper processes, timing-dependent `ps` polling, SQLMesh test database connections and other OS children. Successful app/run subprocess logs are suppressed in the retained output. The model-log cancellation trigger does not establish whether its interrupted native Context was constructed. Report these estimates with their scope; they cannot satisfy a claim of complete observed process inventory.

Cost-accounting cautions:

- CI2’s `pipeline` result, 5.019 seconds, measures only `ci/test models aws-cloudtrail`. It excludes fast checks and selector/setup work; it is not a measured complete pipeline PR workload.
- Shared 41.781 seconds similarly excludes its separately selected fast lane.
- `ci2-fast.log` contains timing JSON from canned Go-event tests nested inside the Python unittest command. Adding those events again double-counts work. Its outer command work is **17.456 seconds**, not 19.083.
- Current hosted cache restore/save, queue, whole-job and PR critical-path costs remain unqualified. November 4 hosted logs are historical.

Read/check evidence: read the approved CI2–CI-R plan and ARC-012/013 requirements, AGENTS.md, ADR-001, CI guide, workflow/action, runner, preparation script, native contract runner, shared app/recovery/reader tests, prior proposal and retained before/CI2 logs. Commands used were read-only `cat`, `nl`, `sed`, `rg`, `shasum`, `git rev-parse`, and Python digest/log summaries. No application checks were rerun. No background work remains.
