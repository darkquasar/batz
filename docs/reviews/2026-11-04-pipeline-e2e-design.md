# Independent review: revised pipeline E2E and shared-fixture design

Reviewer: Claude (Opus 5.5), fresh peer review requested by the owner before implementation.
Date: 2026-11-04. Mode: read-only source inspection. No subagents, no test runs, no source edits.
This report is the only file written.

## Subject and identity

- Root `/Users/martinpopolito/GitHub/batz`, branch `feat/incremental-evidence-pipeline`,
  HEAD `b361019c0dd527cb156dd8697cbf153074e0b670`.
- Dirty/untracked at review time: `M AGENTS.md`, `M contributing.md`, `?? docs/reviews/`,
  `?? docs/usage/pipeline-e2e-proposal.md`, `?? docs/usage/pipeline-e2e.md`. No source drift.
- Subject: the revised design in `docs/usage/pipeline-e2e-proposal.md` (233 lines), with the
  blueprint `docs/usage/pipeline-e2e.md` and the `AGENTS.md`/`contributing.md` diffs as
  consumers. The earlier review `docs/reviews/2026-11-04-pipeline-e2e.md` was read as history only.

Read: ADR-001; the four documents; `internal/app/aws_cloudtrail_e2e_test.go`,
`replay_e2e_test.go`, `synthetic_jsonl_e2e_test.go`, `native_test.go`, `publication_test.go`,
`preview_review_test.go` (helper), `app_runtime_test.go:240-462` (`TestAppSharedRecoveryJourney`);
`internal/app/testdata/synthetic/*`; `models/synthetic-jsonl/*.sql`;
`models/aws-cloudtrail/api_activity.sql:1-40`; `runtime/sqlmesh/runner.py:140-200`,
`inspect_preview.py:120-150`; the test names in `runtime/sqlmesh/test_runner.py`;
`internal/run/normalization.go:170-200`; `ci/select.py`, `ci/test.py`, `ci/pipelines.py`,
`ci/native_select_test.py`, `ci/prepare-macos.py`; `.github/workflows/pr.yml`,
`.github/actions/prepare-macos/action.yml`.

Limits:

- The peer spec repository `../specs/batz` could not be read: both `ls` and file reads outside
  the working directory were refused by session permissions. So I could not check the proposal
  against the CI-simplification spec/plan or the S3.1 budgets. Treat that lens as **unknown**, not clean.
- The spec-review rubric file and the house writing-style file were also outside the permitted
  paths. I applied the rubric summarized in the skill body.
- The hosted CI logs were not re-read. The timing figures are taken from the proposal.

## How coverage would move

```text
  today                                     revised design
  +-----------------------------+           +-----------------------------------+
  | CloudTrail e2e job          |  shrink   | CloudTrail e2e job: blueprint     |
  | 5 tests, 12 native runs     | ========> | 1 run, 1 reader query             |
  +-----------------------------+           +-----------------------------------+
        |  move preview/strict/growing                 ^ pipeline-only PRs
        v                                              |
  +-----------------------------+           +-----------------------------------+
  | shared app lane (batz_app)  | <======== | synthetic e2e job (removed);      |
  | isolated fixture content    |   fold    | assertions into existing 'other'  |
  +-----------------------------+           | run of TestAppSharedRecoveryJourney|
        |  CloudTrail identity/conflict     +-----------------------------------+
        v  and time interpretation
  +-----------------------------+   gap (M1): runner.reconciliation_counts has
  | CloudTrail native SQLMesh   |   no executing proof after the move
  | model tests                 |
  +-----------------------------+
```

## Findings

### M1 (Medium): framework reconciliation accounting loses its only proof

- Evidence: the identity row (`pipeline-e2e-proposal.md:97`) sends CloudTrail identity/conflict
  to native model tests. It says "Generic multi-origin integrity remains covered through the
  production bridge's shared positive/negative tests". That is true for origin membership.
  It is not true for the reconciliation accounting, which is Batz framework code:
  - `runtime/sqlmesh/runner.py:153-171` (`reconciliation_counts`) groups staging rows by
    `logical_event_id`/`norm_event_variant_id`. It produces the `observations`, `variants`,
    `logical_events`, `conflicting_events` and `unidentified_observations` values that are
    published in the run report (`runner.py:309-312`, `internal/contracts/wire.go:142-148`).
  - `internal/run/normalization.go:185-189` enforces the invariants on those counts.
  - The only executing assertion of these values is the CloudTrail growing journey
    (`replay_e2e_test.go:52-53`: 15/12/3/1/8). `runtime/sqlmesh/test_runner.py` has no test that
    calls `reconciliation_counts` or asserts `reconciliation`. No Go test exercises the
    `normalization.go:186` refusal.
  - After the move, the shared journey uses synthetic content whose staging hard-codes
    `logical_event_id: NULL` (`models/synthetic-jsonl/records.sql:2-3`). The identity grouping,
    variant and conflict branches therefore never execute. Native CloudTrail model tests do not
    run `runner.py`. The blueprint does not assert `report.Reconciliation`.
- Consequence: a regression in exact reconciliation accounting, which ADR-001 rule 7 says to
  keep, would pass every selected check. The row's fallback, "retain a source integration
  assertion if lower-layer proof does not cover a distinct boundary", is conditional. It also
  conflicts with acceptance item 1, under which the CloudTrail job "runs that journey only"
  (`:203-206`). As written, an implementer can satisfy the plan and still drop this proof.
- Correction (no new native start):
  1. In the identity row, state that `reconciliation_counts` is framework behavior and name its
     destination: a direct unittest in the existing `runtime/sqlmesh/test_runner.py`. That file
     already runs inside the app lane's single Python contracts interpreter
     (`ci/test.py:162-168`). Use invented staging rows covering a repeated identical variant, two
     conflicting variants of one logical ID, unidentified rows, a non-`mapped` row that must be
     excluded, and a staging table without identity fields (the `NULL::VARCHAR` fallback at
     `runner.py:158-159`). Assert the exact dict.
  2. Optionally add a direct Go check that `normalization.go:186` refuses an impossible count
     (for example `LogicalEvents > Variants`). This is cheap and has no native start.
  3. Remove the conditional fallback, or say that any retained source assertion goes into the
     blueprint's single reader query (for example `report.Reconciliation` for the one-record
     fixture), not into a second run.
  4. Add this test to the "observe replacement pass before deleting" list at `:102-105`.

### M2 (Medium): removing the internal-probe E2E conflicts with current selector and runner behavior, and one fixture-model trigger is missing

- Evidence:
  - The design removes `synthetic_jsonl_e2e_test.go` and its matrix entry (`:129-131`,
    `:203-206`) and keeps probes out of the E2E matrix and the all-local loop (`:147-150`).
    But `:169-170` says helper changes select "the existing `broaden` representative set (first
    production pipeline and first internal probe)". After the removal, the first internal probe
    has no E2E to run.
  - Current code still puts probes into E2E. `ci/select.py:204` sets `e2e = set(adapters)`, and
    adapters include `synthetic-jsonl` whenever it is affected. `ci/select.py:210-212` adds
    `probes[:1]` on broaden. `ci/test.py:221-223` loops over every catalog identity for
    `all-local`. `ci/test.py:183-185` raises `missing requested E2E tests` when the file is
    absent, so the first selected probe or all-local run fails with exit 2.
  - Existing selector regressions encode the old behavior:
    `ci/native_select_test.py:194` (`e2e == ['probe']`) and `:210`
    (`e2e == ['pipeline-a', 'probe']`).
  - Missing trigger: `ci/select.py:135-136` sets `app` only for owned *assets* of a `/testdata/`
    manifest. A change to `models/synthetic-jsonl/*.sql` reaches the selector only through
    native impact (`ci/select.py:180-183`), which adds the owner to `affected` and never sets
    `shared_app`. Today the synthetic E2E job catches this. After it is removed, a fixture-model
    edit selects only model and adapter checks. Every shared journey that consumes those models
    (preview, strict, growing, recovery) would be skipped. The proposal states the rule
    ("Changed internal fixture assets or models select the shared app lane", `:148-149`), but its
    case list (`:182-187`) says "internal fixture changes" without separating the asset path
    (already handled) from the model-impact path (not handled).
- Consequence: implemented literally, the design either fails CI on the first broaden or
  all-local run, or quietly brings back the probe E2E it removes. Without the model-impact
  trigger, the shared fixture models lose their shared-lane coverage, the same kind of hidden
  dependency the revision exists to remove.
- Correction:
  1. Replace `:169-170` with: shared helper and bridge changes select `production[:1]` E2E plus
     `app=true`. Probes never enter `e2e`.
  2. State the selector rule: a `/testdata/` owner reached by asset, native model impact or
     native test sets `app=true` and contributes to `models`/`adapters` only. In `ci/test.py`,
     `all-local` loops only over production identities. An explicit `ci/test e2e <probe>` still
     refuses, which is correct.
  3. Name the selector cases: `models/synthetic-jsonl/findings.sql` only gives `app` true,
     `e2e` `[]` and `adapters` `['synthetic-jsonl']`. Bridge change gives `e2e` equal to the
     first production pipeline only. Update the two existing assertions at
     `native_select_test.py:194,210`.

### L1 (Low): the isolation check would miss direct repository reads, and the helper split is unstated

- Evidence: `:186-187` verifies isolation "by inspecting copied content". Several helpers read
  production assets from the repository directly, so copied content never shows them:
  `sourceRecords` (`native_test.go:39`, `../../pipelines/aws-cloudtrail/fixtures/api-read.json`),
  `previewContent` (`preview_review_test.go:21`), and `runFixture`, which hard-codes the
  CloudTrail manifest (`native_test.go:16`). `nativeContent` copies `pipelines`, `models` and
  `tests` wholesale (`publication_test.go:20`). The blueprint example itself calls
  `nativeContent` (`pipeline-e2e.md:69`), so narrowing that helper would break the template.
  The existing shared journey also writes its "unrelated model must not execute" poison to
  `models/aws-cloudtrail/unrelated.sql` (`app_runtime_test.go:281-282,429`). In an isolated root
  that directory has no manifest, so what the assertion proves may change.
- Correction: say that pipeline E2Es keep a full-content helper and shared `batz_app` tests get a
  separate fixture-only root (config, `runtime/sqlmesh`, fixture models/tests and
  `internal/app/testdata`), built from `copyTree`. Move CloudTrail-shaped helpers next to the
  CloudTrail E2E. Rename the poison to a fixture-owned sibling and confirm it still loads during
  discovery, so the assertion is not vacuous. As a one-time postcheck, grep the `batz_app` test
  files and shared helpers for `aws-cloudtrail` and `../../pipelines`. No permanent guard.
  Also scope the rule: untagged fast-lane tests (for example `preview_review_test.go:32-53`,
  `internal/run/cancel_test.go:16`) use CloudTrail content but run on every non-docs PR. Either
  exempt them explicitly or include them.

### L2 (Low): the strict-partial destination needs fixture-model changes the design does not mention

- Evidence: `:95` asks for invented valid, rejected and unsupported records. The synthetic staging
  classifies every row as `mapped` with a NULL reject reason (`records.sql:2-3`). Today's strict
  test relies on CloudTrail SQL to produce one rejected and one unsupported record
  (`aws_cloudtrail_e2e_test.go:206-208,230`).
- Correction: state that the framework fixture gains a minimal invented reject/unsupported rule
  (for example keyed on an invented `kind` field) with its own native model case. Changing those
  fixture models then selects `app`, as M2 requires. A test-time SQL mutation like the existing
  shared journey's is an acceptable alternative. Name whichever is chosen.

### L3 (Low): the shared time row asks the fixture for source interpretation it does not model

- Evidence: `:100` keeps "nano-distinct, coarse, offset, missing-time and historical
  observations ... with grouped native `epoch_ns`/type/granularity queries" in the internal
  shared journey. The synthetic output is a bare `cast(... AS TIMESTAMP_NS)` with no `batz.time`
  struct (`findings.sql:3`), so it has no offset or `fraction_digits` to assert. Offset and
  granularity are CloudTrail interpretation, already assigned to its native test at `:99`.
- Correction: limit the shared row to framework preservation: `TIMESTAMP_NS` type and exact
  `epoch_ns` for nano-distinct and millisecond values, NULL for undated, and a historical
  instant, kept across replay and the old revision. Move "offset/granularity" to the CloudTrail
  row. This avoids copying source time logic into a framework fixture.

### L4 (Low): runner guard semantics and log readability are underspecified

- Evidence: `:188-191`. The current e2e lane runs plain `go test` (`ci/test.py:186`).
- Correction: define the refusal as follows. Exit non-zero unless at least one top-level `Test`
  event has Action `pass`. Refuse any `skip` event, subtests included. Treat a missing package
  result or `build-fail` as failure. Re-emit each event's `Output` text so CI logs stay readable
  (AGENTS.md requires readable logs). Keep it in `e2e()` only: the app lane's helper tests pass
  by returning early (`app_runtime_test.go:21-24`), and `internal/scratch/macos_test.go:19`
  skips on non-macOS, so applying it there needs separate thought. Exercise the parser with
  canned event streams in `ci/test_jobs.py`. No native start is needed.

### L5 (Low): the cost and cache evidence is missing a before-baseline and a frozen-key caveat

- Evidence: acceptance item 3 (`:214-216`) asks for total work and per-job time after the moves,
  but only the E2E jobs have a recorded before. The app lane, which receives the moved tests,
  has no before time or native-start count. Separately, the build-cache key is
  `hashFiles('go.mod','go.sum')-job-matrix` (`action.yml:20`). An exact-key hit skips the save,
  so the first successful run's `GOCACHE` stays frozen for that PR. A failed or cancelled first
  run (`pr.yml:9-11` cancels in progress) saves nothing.
- Correction: record the `shared-runtime` job time and the local `ci/test app` package time and
  native starts at `b361019` from the existing run, so the moved cost can be computed. Make
  acceptance item 5 report the rerun's collector-build and Go-command seconds, not only
  "restored". Note that later commits in the PR recompile against the frozen cache. No new
  trigger or key scheme is needed.

### L6 (Low): two status and mechanism nits

- `AGENTS.md:11` and `pipeline-e2e.md:123-127` describe isolation in the present tense ("Shared
  app tests use isolated framework-owned fixtures"). `contributing.md:140` correctly says it is
  not implemented yet. Mark the first two as the target state until the moves land, so agents
  do not rely on a dependency separation that does not exist yet.
- Empty-publication row (`:98`): removing the typed empty file before adding A/B depends on how
  acquisition treats a file that disappears from a growing directory. I did not verify this. One
  sentence naming the expected accounting for the removed file would close it.

## Areas checked and found sound

- **Synthetic fold without extra native starts.** This is feasible as described. The `other`
  binding run already exists (`app_runtime_test.go:303`), and its final count-only query
  (`:400`) can become the ordered actor/action/type/epoch query. The input must come from a
  separate path (for example the original `events.jsonl`), because the shared `input` holds one
  line at that point (`:255`) and is overwritten later (`:309`). Exact ordered rows still detect
  the injected uncommitted parquet (`:295-300`), so that guard keeps its meaning. The run uses
  the mutated `findings.sql` (`shared_count` column), which does not change action, actor or
  time. Adapter JSONL fidelity stays with `--mode adapters`.
- **Preview move.** The `fractional-time` branch detection is framework code
  (`inspect_preview.py:135-136`), and the synthetic fixture already contains a nanosecond record,
  so the explicit-example proof moves cleanly.
- **Selection for production pipeline-only changes.** CloudTrail model-only and new-pipeline
  additions already leave `app` false (`select.py:135-145,180-183`). The cases at `:182-184`
  are correct regressions. Runtime, config, CI and dependency paths broaden (`select.py:140-142`).
  The `replay_e2e_test.go` gap (`:171-173`) is accurately described.
- **Cache fix.** The `$RUNNER_TEMP` plus `$GITHUB_ENV` approach in the composite action is valid.
  Later steps, including `select`'s `$BATZ_TOOLCHAIN_ROOT/venv/bin/python` (`pr.yml:53`), see
  the variables. `ci/test.py:43-45` uses `setdefault`, so the CI values win. The same-PR-only
  scope and the excluded default-branch trigger are stated honestly (`:28-33`, `:224-228`).
- **Cost honesty.** The section at `:107-112` and `:155-157` declines to claim savings before
  measurement, and the hosted breakdowns add up (for example 37.236 s minus 6.792 s is about 30 s
  of compilation/startup).
- **Runner zero-test gap.** This is real: `ci/test.py:184-186` checks only the file, the tag and
  the exit status.

## Disposition

Severity counts: 0 Critical, 0 Major, 2 Medium (M1, M2), 6 Low (L1 to L6).

There are no blockers under the "more than two Medium" rule. Both Mediums need either a fix or
explicit owner acceptance as residuals. I recommend fixing them: each is a bounded text change
to the proposal. M1 adds one Python unittest at implementation time and M2 adds selector cases,
neither with a new native start. One batched correction covering M1, M2 and the Lows (fixed or
explicitly declined) should leave the design ready for owner disposition. The CI-simplification
spec/plan cross-check stays open until someone with access to `../specs/batz` does it. This
review does not authorize implementation, commits or publication.
