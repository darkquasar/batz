# Independent final review: implemented pipeline E2E and shared-fixture simplification

Reviewer: Claude (Opus 5.5), fresh independent peer review requested by the owner.
Date: 2026-11-04. Mode: read-only inspection of source, retained logs and peer specs.
No subagents, no test runs, no source edits. This report is the only file written.

## Subject and identity

- Root `/Users/martinpopolito/GitHub/batz`, branch `feat/incremental-evidence-pipeline`,
  base and HEAD `b361019c0dd527cb156dd8697cbf153074e0b670`, plus the dirty/untracked set in
  `/tmp/batz-final-review-inputs.json` (41 files, two marked DELETED).
- Read in full: `AGENTS.md`, ADR-001, `docs/usage/pipeline-e2e-proposal.md`,
  `docs/usage/pipeline-e2e.md`, `contributing.md`, `docs/usage/ci.md`, the fresh design review
  `docs/reviews/2026-11-04-pipeline-e2e-design.md`; `internal/app/{aws_cloudtrail_e2e,
  app_preview,app_replay,app_runtime,native,publication,dependencies,preview_safety,
  preview_review,explain,explain_context,explain_paths,explain_stages,diagnostics,
  preflight}_test.go`; `internal/run/cancel_test.go`; `internal/config/resolve_test.go`;
  `internal/app/testdata/synthetic/{pipeline.yaml,README.md,events.jsonl}`;
  `models/synthetic-jsonl/{records,findings}.sql`; `models/aws-cloudtrail/api_activity.sql`;
  `tests/aws-cloudtrail/test_mapping.yaml`; `tests/synthetic-jsonl/test_records.yaml`;
  `pipelines/aws-cloudtrail/{pipeline.yaml,fixtures/README.md,fixtures/wiring.json,
  fixtures/wiring.expected.json}`; `runtime/sqlmesh/{test_runner.py (to line 150),
  test_inspect_preview.py,inspect_preview.py,test_models.py}`; `ci/{test,select,pipelines,
  native_select_test,test_entrypoints}.py`; `.github/workflows/pr.yml`;
  `.github/actions/prepare-macos/action.yml`.
- Logs read: `/tmp/batz-composed-final.log` (complete), `/tmp/batz-before-e2e-host.log`,
  `/tmp/batz-before-app-host.log` (first 40 lines).
- Peer specs read: `specs/batz/01-dfir-log-pipeline/pipeline-plan.md` (D3, S1, S3.1 sections and
  the S3.1 workload table) and `specs/batz/01-pipeline-architecture/pipeline-architecture-plan.md`
  (layout, T1, T2).

### Limits of this review

- Every Bash call (including `shasum`, `git status`, `git diff`) was refused by session
  permissions, and the serena symbol tools were not granted. So I could **not** verify the
  SHA-256 inventory, read the deleted files' bytes, inspect `git diff`, or grep the tree.
  The only hash I could confirm indirectly is `wiring.json`, whose manifest-declared
  `sha256` equals the inventory value `6625098d...`. I reviewed the bytes the Read tool
  returned for each listed path at review time.
- Because the deleted `replay_e2e_test.go` and `synthetic_jsonl_e2e_test.go` could not be
  read, "coverage preserved" statements below rely on the design review's description of
  the deleted assertions, the proposal tables and the current replacement bytes.
- The one-time "CloudTrail appears only in its own E2E among shared tests" scan was not
  independently repeated (no grep). Every shared Go file I read uses only synthetic content.
- No hosted CI was observed. Hosted claims are taken as the proposal states them.

## How coverage now flows

```text
  production-only change                    core/runtime/helper/fixture change
  +-------------------------+               +-----------------------------------+
  | CloudTrail models/tests |               | internal/{app,run}, runtime/,     |
  | pipelines/aws-cloudtrail|               | config.yaml, testdata, synthetic  |
  +-------------------------+               +-----------------------------------+
        | select.py                                | select.py
        v                                          v
  +-------------------------+   bridge path   +-----------------------------------+
  | models (native+adapter) | <~~~~~~~~~~~~~~ | app lane: synthetic-only roots    |
  | + 1-run CloudTrail E2E  |   adds first    | preview, strict, replay, recovery |
  | app = false             |   production    | + Python reconciliation contracts |
  +-------------------------+   E2E           +-----------------------------------+
        |                                          |
        v  CloudTrail identity/combine/conflict    v  generic accounting, precision,
  +-------------------------+                 +-----------------------------------+
  | tests/aws-cloudtrail    |                 | runtime/sqlmesh/test_runner.py    |
  | native SQLMesh cases    |                 | (real DuckDB, no vendor logic)    |
  +-------------------------+                 +-----------------------------------+
```

## Findings: code and test defects

Severity counts for this section: 0 Critical, 0 Major, 1 Medium, 6 Low.

### M1 (Medium): published CloudTrail conflict flags and combined-row lineage have no executing oracle

- Evidence:
  - The final projection in `models/aws-cloudtrail/api_activity.sql:31-39` sets
    `batz.source_event_id` to NULL for combined variants, projects `batz.conflict` from the
    `ranked` CTE, and attaches `v.origins` from the joined variant row.
  - The replacement native case `tests/aws-cloudtrail/test_mapping.yaml:287-436` asserts the
    `variants` and `ranked` CTEs, but its final `outputs.query` (`:430-436`) asserts only four
    rows with NULL `"@timestamp"`. It does not assert `batz.conflict`, `batz.origins` or the
    NULL `source_event_id` on any published row.
  - The CloudTrail E2E fixture (`pipelines/aws-cloudtrail/fixtures/wiring.json`) has two
    distinct `eventID`s, so `aws_cloudtrail_e2e_test.go:34-47` sees `len(batz.origins)=1`
    and never a conflict. The shared journey uses the synthetic fixture whose identity is
    always NULL (`models/synthetic-jsonl/records.sql:2`), so it cannot exercise conflict.
  - The generic bridge check (`runtime/sqlmesh/test_runner.py:95-118`) proves every mapped
    origin appears exactly once overall. It does not prove which row an origin is attached
    to, nor the conflict flag.
  - The peer plan's settled D3 decision states "Default queries expose all flagged ambiguous
    conflict variants" (`pipeline-plan.md:265`). The proposal's identity row
    (`pipeline-e2e-proposal.md:102`) lists "conflict flags" among the coverage to preserve.
- Consequence: a regression in the final SELECT (for example `'conflict': false`, exposing
  `source_event_id` on a combined row, or joining the wrong `variants` row so origins are
  swapped between published rows while overall membership stays exact) passes every
  selected check. That is a lineage/meaning promise silently weakened, which the owner's
  brief says must be preserved at the cheapest sufficient layer.
- Correction (no new native start): in the same native test, replace the four
  `'"@timestamp"':` rows at `test_mapping.yaml:433-436` with expectations on the `batz`
  column for the four output rows: `same` row with `source_event_id: null`,
  `conflict: true` and origins `[a, b]`; `changed` row with `source_event_id: c`,
  `conflict: true`, origins `[c]`; `d` and `e` rows with `conflict: false` and their single
  origin. `partial: true` narrows columns, not struct members, so write the complete
  deterministic `batz` struct (all of its fields come from the fixture inputs and
  `build_context`). No E2E or shared journey change is needed.

### L1 (Low): bare `ci/test e2e` now always fails on the internal probe

- Evidence: `ci/test.py:258` admits a `/testdata/` identity whenever `args.lane == 'e2e'`.
  With no pipeline arguments, `selected = sorted(catalog)` (`:238`) includes
  `synthetic-jsonl`. `e2e()` then raises `missing requested E2E tests: synthetic-jsonl`
  (`:183-185`), exit 2. The S3.1/S3.2 plans keep `ci/test e2e [PIPELINE...]` with optional
  arguments (`pipeline-plan.md:666-668`, `pipeline-architecture-plan.md:207-209`).
- Consequence: a developer running the documented bare lane to execute all production E2Es
  gets a loud failure for a probe they did not request. CI is unaffected because it always
  passes one identity (`pr.yml:144`).
- Correction: change the condition to
  `'/testdata/' not in catalog[identity]['path'] or identity in args.pipelines`, so only an
  explicit `ci/test e2e synthetic-jsonl` refuses. One cheap case in `ci/test_entrypoints.py`
  is optional.

### L2 (Low): the shared app lane now prints raw Go JSON events in CI logs

- Evidence: `ci/test.py:170` permanently adds `-json` to the shared `./internal/app` command.
  The before log ran plain `go test` (`/tmp/batz-before-app-host.log:3`). The composed log
  shows the shared lane as raw event objects (`/tmp/batz-composed-final.log:26-200`). Unlike
  `e2e()` (`:191-199`), `app()` uses `command()`, which does not re-emit `Output`. The
  proposal said the `-json` capture was for the one composed measurement and that no
  permanent guard is required (`pipeline-e2e-proposal.md:215-217`).
- Consequence: hosted shared-runtime logs, the lane contributors read most on core
  changes, become hard to read. AGENTS.md and the design review's L4 ask for readable logs.
- Correction: drop `-json` from `ci/test.py:170` now that the one-time capture is retained,
  or route that command through the same Output re-emission the E2E lane already uses.

### L3 (Low): one new selector regression does not isolate its second case

- Evidence: `ci/native_select_test.py:241-246` loops over `internal/run/recovery.go` and
  `internal/app/app_replay_test.go` but never resets `self.base`. The second iteration's
  diff therefore contains both paths, so the `app_replay_test.go`-alone case (the gap the
  proposal explicitly names at `pipeline-e2e-proposal.md:174-177`) is only tested together
  with a core file that already selects `app`.
- Consequence: a selector regression that stopped attributing moved shared tests to `app`
  would still pass this test.
- Correction: set `self.base = self.git('rev-parse', 'HEAD')` at the start of each
  iteration, as `test_fixture_model_and_native_test_select_shared_not_probe_e2e` already
  does at `:234`.

### L4 (Low): the new `wiring` explain example and its expectations are never executed

- Evidence: `pipelines/aws-cloudtrail/pipeline.yaml:30-34` registers `wiring` as a second
  explain example with `wiring.expected.json`. The adapter lane verifies only `examples[0]`
  (`runtime/sqlmesh/test_models.py:35-40`). No test I read calls `explain --example wiring`.
  The registration does serve selection ownership (`ci/select.py:97-98`), which is useful.
  I could not verify whether Go preflight validates every declared example checksum.
- Consequence: `wiring.expected.json` is an oracle that nothing runs, so it can drift from
  the fixture unnoticed. Contributors copying the canonical example may also assume that
  every declared example is adapter-verified.
- Correction: keep the example for ownership but drop its `expectations:` key (ownership
  only needs `input`), and delete `wiring.expected.json`. The E2E reader query is the
  fixture's oracle. Alternatively, state in `pipelines/aws-cloudtrail/fixtures/README.md`
  that only the first example is adapter-verified and that the E2E checks `wiring`.

### L5 (Low): small oracle and diagnostic weaknesses in the moved tests

- `internal/app/app_runtime_test.go:408` and `app_replay_test.go:19` dereference
  `other.Catalog` / `empty.Catalog` without a nil check, so a missing catalog panics instead
  of producing a precise failure. Add `|| other.Catalog == nil` to the existing check at
  `app_runtime_test.go:308` and assert the empty run's outcome/catalog before the query.
- `internal/app/app_preview_test.go:195-199` passes vacuously if `report.Samples` is empty.
  Assert `len(report.Samples) > 0` first.
- `internal/app/dependencies_test.go:72-74` still says "requires the real installation
  receipt", but in the fast lane `nativeContent` supplies the inert receipt
  (`publication_test.go:37-64`). Reword the message.
- Consequence: diagnosis quality only; none of these hides a passing-but-wrong outcome
  except the vacuous sample loop, which the `unobserved` check at `:186-194` partly covers.

### L6 (Low): a README edit in the fixture directory selects the whole shared lane

- Evidence: `ci/select.py:93-94` makes the whole `internal/app/testdata/synthetic` directory
  owned, and `:131` treats an owned `.md` file as non-docs, so the new
  `internal/app/testdata/synthetic/README.md` routes prose edits to `app`, synthetic models
  and adapters.
- Consequence: cost only (a docs edit pays the most expensive lane). Not a safety issue.
- Correction (optional): treat owned `.md`/`.rst` files under a `/testdata/` directory as
  docs unless the manifest references them explicitly.

## Findings: documentation and measured-cost honesty

Severity counts for this section: 0 Medium, 3 Low.

### D1 (Low): the shared-lane before/after comparison is not like-for-like

- Evidence: the table at `pipeline-e2e-proposal.md:317-327` compares the shared Go command
  17.422 s (a standalone `ci/test app` lane) with 42.856 s measured inside `all-local`,
  where fast, models and E2E jobs ran concurrently (`ci/test.py:248-260`; interleaving is
  visible at `/tmp/batz-composed-final.log:26-200`). The text adds this caveat only for the
  fast Go command (`:336-337`).
- Consequence: the after figure is inflated by contention. The conclusion still holds: the
  four moved tests alone take 6.31 + 2.69 + 3.38 + 13.85 = 26.23 s of package time
  (`composed-final.log:49,53,81,158`), above the 25 s limit (20 s plus 25%).
- Correction: add one sentence saying the shared-lane after times are composed/contended,
  and that the moved tests' own 26.2 s already exceeds the tolerance.

### D2 (Low): the per-PR-class cost shift is not stated

- Evidence: under the base selector a core-only change (for example `internal/run/*.go`)
  selected `app` with `e2e=[]` (consistent with the design review's description of
  `select.py` and the pre-existing `test_shared_app_selects_critical_runtime_without_catalog_e2e`).
  The moved CloudTrail journeys therefore did not run for core-only PRs before. They now do.
- Consequence: the aggregate "about 1.6% lower" total hides a redistribution. Pipeline-only
  PRs lose about 28 s of E2E package time; core-only PRs gain about 25 s of shared Go
  command time, along with real coverage they previously lacked (the replay gap the
  proposal names). Readers may otherwise assume no class got slower.
- Correction: one sentence in the measured-limits section naming both directions.

### D3 (Low): wording nits

- `pipeline-e2e-proposal.md:275` says the canonical E2E is 44 lines; the file has 49.
- `contributing.md:65` "Production skips contributor model tests" is unclear; for example,
  "Production runs never execute contributor model tests" (if that is the intended meaning).
- `ci/native_select_test.py:198` is named `..._and_testdata_probes` but now asserts that
  probes never enter E2E; renaming avoids misleading future readers.

## Acceptance evidence still missing (not code defects)

These items do not count toward the code-defect severities above. They need either a
measurement or an explicit owner decision.

- **E1, shared-app budget miss (disclosed).** S3.1 requires an ordinary shared-app PR within
  20 s (plus 25%) (`pipeline-plan.md:700-708`). It is missed, as the proposal states
  (`:347-354`). Under S3.1 a miss returns measured cost plus the next simplification to the
  owner. The proposal names that simplification. **Owner acceptance is required.**
- **E2, the headline workload was not measured as a fixed workload.** S3.1's "one changed
  local pipeline including representative E2E, <=20 s critical path" (`pipeline-plan.md:704`)
  and the fast workload (<=5 s, 6.5 s accepted) have no standalone after-measurement. Only
  composed components exist: fast Go 6.48 s, models 1.793 s, E2E command 5.362 s, collector
  build 2.688 s. The before value for the pipeline workload was the 36.348 s E2E lane
  (`batz-before-e2e-host.log:9`). The components suggest the target is now met, but that is
  inference. Either measure `ci/test fast`, `ci/test models aws-cloudtrail` and
  `ci/test e2e aws-cloudtrail` once as the workload, or record it explicitly as unmeasured.
- **E3, start counts cover only changed journeys (disclosed).** S3.1 asks to count all
  selected processes/engine starts (`pipeline-plan.md:711`). The proposal reports 13 to 12
  invocations and 11 to 10 model executions for affected journeys only, and says so
  (`:339-345`). I re-derived both counts from the current tests and they match.
- **E4, hosted results unmeasured (disclosed).** No push or hosted run, so cache-save warning
  removal, same-PR cache restore, whole-job times and PR critical path are unknown. One
  mechanism is also hosted-only: `${{ env.* }}` in the composite action's `actions/cache`
  `with:` relies on values written to `$GITHUB_ENV` by the preceding composite step
  (`action.yml:10-27`). That is the documented pattern, but only a hosted run proves it here.
  **Owner decision on hosted qualification.**

## Areas checked and found sound

- **Fixture isolation.** `nativeContent` (`publication_test.go:15-72`) copies only
  `config.yaml`, `runtime/sqlmesh` and the synthetic manifest/models/tests by default. The
  CloudTrail E2E passes its own three paths (`aws_cloudtrail_e2e_test.go:16`). Every shared and
  untagged test I read, including fast preflight/explain tests, `internal/run/cancel_test.go`
  and the preview expectation unit test, uses only invented synthetic content. Poison models
  live under `models/foreign/`, and `TestAppUndeclaredModelSelectionRefusedBeforeAcquisition`
  shows that directory is discovered in the isolated root, so the poison is not vacuous.
- **Synthetic fold.** The `other` run (`app_runtime_test.go:307-310`) uses the original
  two-row JSONL. One grouped query (`:407-410`) checks action, actor, `TIMESTAMP_NS`, exact
  ns/ms epochs, retained payload, origin count/id and parsed revision. No run or model
  execution was added. The uncommitted-parquet guard keeps its meaning through exact rows.
- **Replay journey.** `app_replay_test.go` covers typed empty publication, then removal, then
  new/new/changed/replay/no-op, with acquisition accounting at each step. It checks exact
  ns-distinct, coarse, historical and undated values through replay and in the immutable
  first revision, generic reconciliation with no identity (13/13/0/0/13), and zero
  collection/model starts on the no-op. The counts are internally consistent.
- **Strict partial and preview.** These use invented `disposition` rows plus a `null` record,
  real handoff-environment assertions, the durable report, the read-only context and a
  model-free strictness retry. The preview keeps workspace/binding selection, macros,
  lossless structures, repeatability and both inspection-failure classifications.
  `fractional-time` detection is generic (`inspect_preview.py:134-136`).
- **Source-owned identity and time.** CloudTrail native cases cover scoped/absent/malformed
  identity, repeated and changed payload variants, nano-Z/nano-offset/coarse/undated/
  malformed/too-precise/out-of-range time, and semantic/IP expectations. Framework
  reconciliation accounting, including the no-identity schema, is in `test_runner.py:71-89`
  (closes the design review's M1). Optional identity cannot block: the first replay run
  asserts complete quality with NULL identity.
- **Blueprint E2E.** One fixture, one `run`, one reader. The grouped exact rows check
  meaning, native time type/value, offset and fraction digits, retained payload and origins
  tied to the reported parsed revision. Its 2.68 to 3.64 s body is within the 10 s target.
- **Runner guard.** `ci/test.py:182-221` refuses no executed top-level pass, any selected
  skip including subtests, a missing package pass and any run/pass mismatch. It re-emits
  readable Output. The canned-stub tests exercise all four branches.
- **Selection.** Production-only and E2E-only changes give `app=false`; core and moved shared
  tests give `app` without E2E; bridge helpers (all three helpers the E2E uses are listed at
  `select.py:123-127`) give `app` plus `production[:1]`; fixture assets, models and native tests
  give `app` with no probe E2E; `all-local` excludes probes (`ci/test.py:258`).
- **Contributor docs.** `contributing.md` is concise and example-driven. It links the executed
  Go file and does not duplicate a snippet. It tells contributors not to copy shared
  matrices. `pipeline-e2e.md` states the runner's limits honestly. No new registry, framework,
  fixture cache or coverage ledger was introduced.
- **Cost reporting.** The all-local 45.658 s, the component times and the sum 85.281 s all
  match `/tmp/batz-composed-final.log` (I re-added the sum exactly). The 1.6% total is
  correctly called noise. Hosted claims are explicitly withheld.

## Disposition

- Code/test defects: 0 Critical, 0 Major, **1 Medium (M1)**, 6 Low (L1 to L6).
- Documentation/cost honesty: 3 Low (D1 to D3).
- Acceptance evidence: E1 (shared budget miss) and E4 (hosted) need explicit owner
  decisions. E2 needs one cheap fixed-workload measurement or an explicit "unmeasured"
  record. E3 is disclosed.

No blocker under the rule that more than two Medium findings block. M1 needs a fix in the
one bounded correction pass, or explicit owner acceptance as a residual. I recommend fixing
it: it is a native-test expectation change only, with no new native start. L1, L3 and the
D-items are one-line edits. L2, L4 and L6 are small and optional. This review does not
authorize commits, publication or hosted runs, and it does not request another review wave.

## Parent disposition: one bounded correction pass

The complete reviewer findings above are retained unchanged. The reviewer used
Read/Write successfully but its Bash/symbol requests were denied by normal Claude
permissions despite the lead launching with source, peer-spec and report access.
The parent independently checked the supplied input hashes and the one-time source
scan. The supplied review inventory contained 42 entries (including two tracked
DELETED entries); the report's 41-entry count is a transcription error. Corrected
bytes are parent-verified, not independently re-reviewed.

- **M1 — fixed.** Replace CTE-only expectations with complete final `batz` structs,
  expressed as ordinary native expected SQL. One case has two conflicting combined
  variants with exact two-origin lists and NULL `source_event_id`; the other has
  two unidentified single-origin observations with false conflict flags. Splitting
  the cases avoids SQLMesh's mixed NULL/string struct sorting limitation, without
  a helper model, new engine start or application run. It also reduces the native
  fixture file from 436 to 361 lines. `ci/test models aws-cloudtrail --mode native`
  passed all ten selected tests, including exact selected/executed identities:
  `/tmp/batz-final-lineage-green.log` (exit 0, 1.530 s command).
- **L1 — fixed.** Only explicitly requested internal probes enter the E2E loop;
  bare E2E and all-local select production E2Es. A cheap scheduler regression
  observed the old unintended probe selection (RED), then passed (GREEN), including
  explicit-probe refusal: `/tmp/batz-bare-e2e-{red,green}.log`.
- **L2 — fixed.** Restore ordinary readable shared Go output; JSON remains only
  in the E2E runner and the retained one-time composed evidence.
- **L3 — fixed.** Advance the selector fixture base before each core/replay case,
  proving each changed path independently.
- **L4 — fixed.** Remove the unexecuted `wiring.expected.json` and its manifest key.
  Retain the example input for ownership; its executing oracle is the E2E reader.
- **L5 — fixed.** Add explicit catalog/publication and nonempty sample checks;
  correct the receipt diagnostic. The three affected real shared journeys passed
  again in the correction checks.
- **L6 — fixed.** Directory-owned fixture notes are docs-only unless a manifest
  explicitly references that file. Public selector checks observed RED and GREEN
  and retain the existing explicitly referenced Markdown evidence case:
  `/tmp/batz-fixture-docs-{red,green}.log`.
- **D1/D2/D3 — fixed.** State contention and per-PR-class cost shifts, explicitly
  mark standalone after fast/one-pipeline workloads unmeasured, correct 49 lines,
  clarify production runs, and rename the selector regression.
- **E1/E4 — remain open owner decisions.** Shared-app budget acceptance and hosted
  cache/whole-job/PR-critical-path qualification are not claimed complete.
- **E2 — disclosed unmeasured.** No additional benchmark campaign or repeated full
  suite was run. The composed components are observations, not fixed standalone
  after-workload measurements.
- **E3 — disclosed scope.** Counts remain changed-journey counts, not a whole-suite
  OS process/start inventory.

Fresh correction composition used the existing runner and its two native slots,
with one private collector build: `/tmp/batz-correction-check.py` (an ephemeral
invocation, not repository infrastructure). Full log:
`/tmp/batz-correction-check.log`, exit 0. It composed fast checks, tagged vet,
all 22 selector tests and the three affected shared journeys. The native lineage
suite above separately covers the source expectation correction. No application
runtime/production model implementation changed, no safety assertion was removed,
and no second all-local or independent review wave was launched. Dependency
Pydantic/fork deprecation warnings remain in the retained log; checks passed.

Final source/report inventory and hashes: `/tmp/batz-delivered-inputs.json`.
No commit, push or hosted run was performed.
