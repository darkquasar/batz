# S3.2-CI execution evidence — 2026-11-05

Scope: CI1–CI5 only, with CI-R/aggregate implementation review to follow. No S5 work.

Root `/Users/martinpopolito/GitHub/batz`; base/HEAD `b361019c0dd527cb156dd8697cbf153074e0b670` plus existing dirty/untracked cleanup. All 140 captured source hashes matched. No user changes were discarded. The existing plan owns the serial mode, CI1 replacement table and CI4 synthesis. No commit, push, hosted run or cleanup was performed.

```text
+-----------------+  actual input  +-------------------+
| Source contracts| =============> | Adapter + native  |
+-----------------+                | roots + validators|
                                   +-------------------+

+-----------------+  invented input +------------------+
| Shared app proof| ==============> | Run to reader    |
+-----------------+                 +------------------+
```

Changes: the existing synthetic recovery journey is the representative shared run-to-reader proof. Its fixtures cannot load vendor assets; focused safety, preview, strictness and replay checks remain. Actual adapter output/schema now feeds every declared native root and mandatory audit, with production execution/profile/build/origin/accounting functions and executed fixture expectations. Native type/epoch queries execute. Source tests retain final-row lineage and distinct UTC/offset nanoseconds. Production E2E files/tags/lane/matrix/ownership are retired. Contributor/agent/ADR guidance uses native contracts.

Strategy: green-coverage proof moves; critical-tdd for selection, runner refusal, native contract required-check failures and audited shared-input refresh; focused-postcheck for docs/cache wiring. No new application runs were added to the representative journey. Two small native integration regressions use actual adapters; cheap schema/selection/Go-event controls cover invalid, missing, omitted and skipped selected work.

CI1 baseline: unchanged shared `ci/test app` passed on the qualified host in 44.142 s (65.262 s outer command work). Initial restricted-host process/sandbox failures are retained and are not qualification. Initial `ci/test fast` passed in 6.988 s. The combined pipeline baseline failed two existing query-fixture type inferences (`KeyError: caller`); split native/adapter checks passed, but its E2E refused the already-invalid missing `wiring` expectations reference. There is no valid before pipeline/all-local workload and no fabricated speed comparison.

CI2 post-decoupling/pre-optimization observations: combined CloudTrail 5.019 s; independent native 1.261 s; shared app 41.781 s/64.590 s outer command work; fast 9.885 s/17.456 s outer work. These are individual lanes, not complete PR paths. Input hashes and logs are retained in `/tmp/batz-ci-follow-on/ci4-inputs.json`, `ci4.patch`, `ci2-results.json` and `ci2-*.log`.

CI4 reports: [selection](2026-11-05-ci4-selection.md), [scheduling](2026-11-05-ci4-scheduling.md), [caching](2026-11-05-ci4-caching.md). All three native leaves settled with no descendants or source drift. The three original Medium findings were corrected in one bounded CI5 batch: production-schema/version/project validation; one native plan-builder/apply operation including new owner models and shared restatement; attributable YAML-test-only edits select only native tests. Cache changes skip unused native-only Go caches and remove adapter identity from the identical collector build key. Three redundant reader starts were removed; assertions use a grouped recovered-catalog query plus the already-held old reader. The mock scheduler selection test no longer waits on real host slots; that concrete delay justified one affected composed remeasurement.

Observed intended RED/GREEN logs: `selection-{red,green}.log`, `runner-{red,green}.log`, `shared-required-red.log`, `contract-red.log`, `ci5-schema-{red,green}.log`, `ci5-selection-{red,green}.log`, `ci5-refresh-{red,green}.log`. Schema RED had three assertion failures, YAML-test-only RED two assertion failures; shared refresh RED failed application of the second owner's audited plan after the first succeeded. GREEN passes the distinguishing 2-row to 1-row transition. The pinned `always_include_local_changes` option belongs to `plan_builder`, not `plan`; the initial TypeError is retained separately in prior tool output. Corrected code uses `plan_builder(...).build()` then one `context.apply`.

Final composed input SHA256: `7a5c6ca3318f9de6a462c8a6ae74e7eec56185c43b70173160e550f6180fb884`. This inventory excludes review-output documents; it includes all executable and contributor inputs. Exact commands, scheduling, actual exits and readable full logs are in `/tmp/batz-ci-follow-on/delivered-results.json` and its referenced files.

| Fixed selected local workload | Commands | Wall s | Outer command work s | Exit |
| --- | --- | ---: | ---: | ---: |
| fast | `ci/test fast` | 6.088 | 9.584 | 0 |
| pipeline | `ci/test fast` + `ci/test models aws-cloudtrail` (concurrent) | 4.276 | 10.542 | 0 |
| core | `ci/test fast` + `ci/test app` (concurrent) | 39.555 | 68.511 | 0 |
| global | `ci/test all-local` | 41.140 | 75.659 | 0 |

These observations include each command's collector build and uncached Go test execution (`-count=1`). Dependency provisioning/downloads, an empty-cache build campaign, selection-job preparation and hosted queue/VM/job/PR wall time were not measured. Collector setup is separately logged: pipeline 1.213 s, core 0.854 s. Native graph selection is verified by real base/head fixtures; this is not a complete hosted PR cost measurement. No timing assertion was added.

Fast 6.088 s fits the accepted 6.5 s bound; pipeline 4.276 s and all-local 41.140 s fit their targets. Core/shared 39.555 s misses 20 s +25% =25 s and remains an owner decision. Do not attribute timing differences across contended workloads to optimization. Retiring the pipeline app E2E removes its app/discovery/publication/reader boundary structurally; its failed baseline supplies no passing cost subtraction. Reader grouping removes three starts; cache changes have no hosted speed qualification.

PR-class selection: docs-only selects whitespace only; a private pipeline or shared SQL without a synthetic runtime consumer selects fast + affected native/adapter contracts, so the single-consumer workload has the measured pipeline command shape. Core-only selects fast + app. Shared SQL consumed by the synthetic journey additionally selects its actual native consumers; global/unknown inputs broaden to fast + app + full affected native/adapter coverage, represented by all-local. Native YAML test-only edits, including synthetic, select native tests only. Source/core deletions, removed shared edges, new independent pipelines and mixed edits have focused selector controls. Shared-SQL command-shape equivalence is not another independently measured wall time.

Independent native mode (`ci/test models --mode native`) passed all 12 selected tests in 1.261 s; its relevant runner/models/tests/schema bytes are unchanged by the later mock scheduler correction. All-local also executed the same 12 test identities plus both actual adapter contracts. The independent `example-jsonl` walkthrough, `/tmp/batz-ci-follow-on/new-pipeline-walkthrough.py`, uses a temporary production-style manifest/models/YAML fixture bundle, independently passes native and actual adapter modes, and adds no Go app test or registry. Full log `new-pipeline-walkthrough.log`, exit 0. YAML/Python syntax, local links and whitespace passed focused checks.

Native/process count scope: in the final shared lane, source-audited counts are 35 collector invocations (16 app +10 direct-run +9 native integration fixtures), 95 completed Context creations plus 0–1 interrupted cancellation Context, nine ordinary/held readers and five preview inspection processes. Of those Contexts, 58 are directly visible in the Python suite log (52 graph discovery, two runtime, two native-regression and two adapter); app/direct-run subprocess logs are suppressed. All-local adds six collectors and one model-test Context for its two selected actual adapters: 41 collectors, 96 completed Contexts plus the interrupted uncertainty. A single-pipeline combined models command has three collector invocations, one Context and one applying plan; native-only mode has one Context and no collector. A native-impact selector adds two discovery Contexts; docs-only/core-without-broadening add none. Fast has five outer commands and no native engine Contexts; app has one collector build plus three outer test commands; all-local has ten outer build/test commands. Native tests run in-process with per-test private DB connections, not extra SQLMesh Context objects.

These are source-audited native counts corroborated where logs expose them, not a complete observed OS-process inventory. They exclude compiler subcommands, sandbox probes, Go helper processes and timing-dependent ps polls. Baseline source estimates were 26 collectors, 85 completed Contexts +0–1 interrupted, 12 ordinary/held readers and five inspectors. Added counts prove new native integration/selection gaps; no full before/after inventory claim is made.

Archive restoration failed normally with 1Password authorization timeout (exit 1). Full reports therefore use the plan's `docs/reviews/` fallback; the partial spec archive/index and parent archive handoff were not edited. Hosted cache restore/save, same/new-PR and remote/platform qualification remain unmeasured. Pinned dependencies, checksums, offline tests, current-source collector builds and two native host slots remain. No data-preservation, ownership/cancellation, sandbox, publication, optional-identity or promised real-transport guarantee was retired.

Task-changed files relative to the initial captured checkout (separate from preserved pre-existing edits):

- `.github/actions/prepare-macos/action.yml`
- `.github/workflows/pr.yml`
- `AGENTS.md`
- `README.md`
- `ci/native_select_test.py`
- `ci/pipelines.py`
- `ci/select.py`
- `ci/test.py`
- `ci/test_entrypoints.py`
- `ci/test_jobs.py`
- `contributing.md`
- `docs/adr/0001-test-at-the-cheapest-sufficient-layer.md`
- `docs/usage/ci.md`
- `docs/usage/pipeline-e2e-proposal.md`
- `docs/usage/pipeline-e2e.md`
- `internal/app/app_runtime_test.go`
- `internal/app/aws_cloudtrail_e2e_test.go`
- `internal/app/publication_test.go`
- `pipelines/aws-cloudtrail/fixtures/README.md`
- `pipelines/aws-cloudtrail/fixtures/wiring.expected.json`
- `pipelines/aws-cloudtrail/pipeline.yaml`
- `runtime/sqlmesh/test_models.py`
- `runtime/sqlmesh/test_scope.py`
- `tests/aws-cloudtrail/test_mapping.yaml`

Evidence remains readable in `/tmp/batz-ci-follow-on/`. Review outputs and bounded final dispositions will be appended; no independent review completion is claimed by this record.

## CI-R results and final parent verification

The [complete native aggregate review](2026-11-05-ci-aggregate.md) and
[complete Claude CI-R review](2026-11-05-ci-r-claude.md) retain their original
findings plus one bounded parent disposition. Native reported one Low; Claude
reported one Medium and seven Low. The original Medium exact-time gap was fixed
using existing fixture expectations: two forms of the same instant are asserted
exactly at native JSON serialization, with independent distinct 1 ns/2 ns native
cases retained. The correction observed RED exit 4, then GREEN exit 0. All 26
selector tests and fast checks also pass; final fast lane is 6.009 s. There is no
unresolved Critical/Major/Medium code finding. Corrected bytes are parent-verified,
not independently re-reviewed; no further review wave was launched.

Each full report's parent disposition contains every source ID, actual correction
commands/logs, old/new hashes, the documented self-contained synthetic-fixture
constraint, a costed preview-consolidation candidate for the still-open shared
budget, and labelled historical E2E costs. The final bounded correction adds
`pipelines/aws-cloudtrail/fixtures/wiring.json` to the prior 24-file task delta.
The reviewed all-local/core costs remain pre-correction observations; changed
adapter/selector/fast inputs have fresh focused checks, and unchanged native/app
proof is reused. No second full suite was required for documentation bookkeeping.

Shared/core 39.555 s versus the 25 s bound still needs owner disposition. The
concrete next candidate saves an estimated 2.4 s by folding the explicit-route
preview into an existing preview execution, preserving its assertions; roughly
37.2 s would still miss. This estimate is not an implementation or speed claim.
Hosted evidence and archive handoff sync remain unavailable. Execution/review
work is retained for owner acceptance; no commit or publication is implied.
