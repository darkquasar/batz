# Independent Claude review: minimal pipeline E2E blueprint proposal

Reviewer: Claude (Opus 5.5), fresh independent peer review requested by the owner.
Date: 2026-11-04. Mode: read-only inspection plus cheap focused reads. No subagents.

## Subject and identity

- Root: `/Users/martinpopolito/GitHub/batz`, branch `feat/incremental-evidence-pipeline`.
- HEAD verified with `git rev-parse HEAD`: `b361019c0dd527cb156dd8697cbf153074e0b670` (matches inputs).
- `git status --short`: only `M AGENTS.md`, `M contributing.md`, `?? docs/usage/pipeline-e2e-proposal.md`,
  `?? docs/usage/pipeline-e2e.md`. No core/source drift, so the 15 tracked source files in the inputs
  JSON equal their HEAD blobs.
- **SHA-256 hashes: not verified.** Every hashing command (`shasum`, `openssl dgst`, a `python3 -I`
  hashlib script) needed a permission approval this non-interactive session could not get. I report
  this as unverified, not as a mismatch. Identity rests on the HEAD SHA plus the clean status above.
  The owner can confirm with:
  `python3 -I -c "import hashlib,json;d=json.load(open('/private/tmp/batz-blueprint-review-inputs.json'));[print(hashlib.sha256(open(d['root']+'/'+k,'rb').read()).hexdigest()==v,k) for k,v in d['sha256'].items()]"`
- Example identity: `/private/tmp/batz-blueprint-example.go` matches the code block at
  `docs/usage/pipeline-e2e.md:49-101` byte for byte apart from the test function name (checked by
  reading both).

## What I checked

Read in full: the four changed documents (`git diff` for AGENTS.md and contributing.md),
`internal/app/aws_cloudtrail_e2e_test.go`, `replay_e2e_test.go`, `synthetic_jsonl_e2e_test.go`,
`native_test.go`, the helper heads in `publication_test.go` and `preview_review_test.go`,
`ci/test.py`, `ci/select.py`, `.github/workflows/pr.yml`, `.github/actions/prepare-macos/action.yml`,
ADR-001, `pipelines/aws-cloudtrail/pipeline.yaml`, and the time section of
`tests/aws-cloudtrail/test_mapping.yaml`. I grepped for the cited test names and identity/conflict
SQL. Evidence logs read: `batz-blueprint-example.log`, `batz-blueprint-e2e-host-baseline.log`, and a
grep of `batz-latest-ci.log` for cache results and warnings. Two GitHub docs pages were fetched to
check context availability and cache scope.

Not run: any test suite, selector or Go command. None was needed to decide the findings below.
Not read: `ci/native_select_test.py`, `runtime/sqlmesh/test_models.py`, `test_runner.py`, the
previous-run CI log, the nested-sandbox failure log, and the plan-reviewer rubric file (its read was
not permitted, so I applied the rubric in the skill body).

## How coverage moves under the proposal

```text
  current: CloudTrail e2e job (tag batz_aws_cloudtrail, -run ^TestE2E)
  +--------------------------------------------------------------+
  | preview x2 | explicit example | strict+retry | local 2 runs  |
  | growing-directory 5 runs (replay_e2e_test.go)                |
  +--------------------------------------------------------------+
        |                    |                     |
        | move               | replace             | move (+ empty run)
        v                    v                     v
  +-------------------+  +---------------------+  +-------------------------+
  | shared app lane   |  | CloudTrail e2e job  |  | shared app lane         |
  | batz_app/TestApp  |  | blueprint: 1 run,   |  | growing journey: 6 runs |
  | preview, strict   |  | 1 reader query      |  |                         |
  +-------------------+  +---------------------+  +-------------------------+
        ^                                                   ^
        |  selector: app=true only for internal/app/* or    |
        +--- testdata owners today; CloudTrail assets/models do not set it (M2)
```

## Findings

### M1 (Medium): the "native mapping tests cover interpretation" claim is false for two dropped time cases

- Evidence: `docs/usage/pipeline-e2e-proposal.md:64` says the growing journey plus "native mapping tests
  cover interpretation" of the removed nano/coarse/offset/missing-time matrix. The removed assertion at
  `internal/app/aws_cloudtrail_e2e_test.go:338` checks `batz.time.fraction_digits` (9 for nanos, 3 for
  coarse, NULL for undated) and a nanosecond value under a `+10:00` offset
  (`2026-10-25T22:00:00.000000001+10:00`, line 282). What survives:
  - `replay_e2e_test.go:58` checks `fraction_digits` for one millisecond `+02:00` record only.
  - `replay_e2e_test.go:61` checks nanosecond-distinct epoch values but not `fraction_digits`.
  - `tests/aws-cloudtrail/test_mapping.yaml:34-62` (`test_time_classification_and_offset`) has a
    whole-second `+02:00`, undated, malformed, 10-digit (too precise) and out-of-range row. It has no
    9-digit fraction, no nanosecond-with-offset row, and does not select `fraction_digits`.
- Consequence: after the removal, nothing tests CloudTrail's nine-digit `fraction_digits`
  classification or nanosecond offset normalization. ADR-001's 2026-11-04 amendment
  (`docs/adr/0001-...md:93-95`) assigns source time interpretation to the pipeline's own evidence.
  This is precision quietly lost, which is exactly what the owner asked to rule out.
- Correction: before the removal, add two rows to the existing native
  `test_time_classification_and_offset` (a `.000000001Z` and a `.000000001+10:00` instant) and add
  `fraction_digits` to its expected columns. That costs nothing at runtime: it is the same batched
  native suite, with no new engine start. Then correct the table row at proposal line 64 to name that
  test. No E2E change is needed.

### M2 (Medium): the selector plan cannot be "narrow" as written, and one current gap is missing from its case list

- Evidence: `ci/select.py:135-136` sets `shared_app` for an owned asset only when the owner's manifest
  is under `/testdata/`. CloudTrail model files (`models/aws-cloudtrail/*.sql`) are not in
  `ownership()` (`ci/select.py:88-100`). They reach `affected` only through native impact
  (`ci/select.py:180-183`), which never sets `app`. The `app` output is a single boolean that runs the
  whole shared lane: `ci/test.py:161-173` runs `-tags batz_app ./internal/app -run ^TestApp`, plus
  `./internal/run ./internal/scratch`, plus the Python contracts. In the hosted run the
  `shared-runtime` job took about 158 s (07:11:13 to 07:13:51 in `batz-latest-ci.log`).
  Proposal lines 86-92 require shared coverage to be selected when CloudTrail assets "are consumed
  there", and also "keep this narrow ... without running every pipeline E2E". The listed selector
  cases (line 91-92) are source-only, shared-helper and shared-fixture. A CloudTrail model-only SQL
  change is named in prose ("changed model semantics", line 89) but not as a case.
- Consequence: if the tests move to `batz_app` and only the listed cases are added, a PR touching only
  `models/aws-cloudtrail/api_activity.sql` (where conflict is computed, line 15) would run the
  blueprint E2E and model tests, but not the preview, strict or growing/replay/conflict journey. That
  is a new coverage gap. If the fix is "set `app=true` for CloudTrail", every CloudTrail-only PR gains
  the whole shared lane (about 2.5 min of hosted wall time on a parallel runner). That is a real cost
  move the proposal does not state, and it sits awkwardly with "keep this narrow". The helper bullet
  (line 83-85) also does not say which E2E a helper change selects: all pipelines, or the
  representative `production[:1] + probes[:1]` already used by `broaden` (`ci/select.py:210-212`).
- Correction (minimal, with no registry):
  1. State the rule concretely. CloudTrail-owned assets **and** native-impact on CloudTrail models set
     `app=true`. Implement this as one constant beside `bridge` (for example
     `REPRESENTATIVE = {'aws-cloudtrail'}`), checked after the impact loop.
  2. Say that helper changes select the same representative E2E set as `broaden`, not every pipeline.
  3. Add the fourth selector case: a CloudTrail model-only change selects `app`.
  4. In the cost section, say that CloudTrail-only PRs will now also run the shared lane. Give its
     measured size after the move.

### M3 (Medium): the "working CI caches" reduction is overstated for contributors, and the example path cannot work where it is suggested

- Evidence: the cache warnings are real. `batz-latest-ci.log:457,867,873,...` shows
  `Invalid pattern '/Users/runner/work/batz/batz/../batz-cache/...'`, and every job shows
  `Cache not found`. But `.github/workflows/pr.yml:3-4` triggers only on `pull_request`. GitHub's cache
  docs say caches created by a `pull_request` run are scoped to its merge ref and "can only be restored
  by re-runs of the pull request ... cannot be restored by the base branch or other pull requests".
  Nothing ever saves a default-branch cache. The build-cache key is also split per job and matrix
  entry (`prepare-macos/action.yml:20`). Separately, the suggested
  `${{ runner.temp }}/batz-cache` (proposal line 98) cannot go in the current workflow-level `env`
  block (`pr.yml:15-21`): GitHub allows only `github, secrets, inputs, vars` there, and `runner` is
  also unavailable in `jobs.<id>.env`. It works only at step level.
- Consequence: proposal line 72-73 lists "working CI caches" among "the real reductions". With the
  path fix alone, every new PR's first run stays cold (51 s dependency prep plus 70 s collector build
  plus about 40 s test compile in the CloudTrail job). Only re-runs of the same PR could benefit. The
  owner could approve expecting a contributor-wait reduction that will not appear. Acceptance item 5
  ("whether a subsequent run restores") would observe a same-PR re-run and could read as success.
  Separately, an implementer who follows the example literally gets a workflow expression error, which
  costs a CI cycle.
- Correction: in the proposal, (a) say the path fix restores only same-PR re-runs, and that cross-PR
  warm hits need a default-branch cache, which is a separate owner decision (for example a `push` to
  `main` trigger that runs the build-cache jobs). Do not imply it. (b) Replace the example with
  computing the paths at step level in the composite action (write `$RUNNER_TEMP/batz-cache/...` to
  `$GITHUB_ENV` before the cache steps). Do not move the cache under the workspace: `ci/test.py:141`
  would scan its untracked files in `whitespace()`. (c) Make acceptance item 5 name which re-run counts:
  a same-PR re-run, or a new PR.

### L1 (Low): "one reader invocation" for multiple datasets has no pattern with the existing helper

- Evidence: `pipeline-e2e.md:11,47` and `AGENTS.md:5-7` ask for grouped multi-dataset checks in one
  reader process. `query()` (`internal/app/native_test.go:61-70`) executes one statement and returns
  one `fetchall()`, and DuckDB returns only the last statement's result. "Declared datasets"
  (`pipeline-e2e.md:27`) is also undefined: for CloudTrail it means `routes.*.output.normalized.models`
  (only `api_activity`), not `staging`/`evidence`/`checks`.
- Consequence: fan-out and multi-output contributors will either invent a combining query or call
  `query` once per dataset and wonder whether that breaks the rule.
- Correction: add one sentence. Either combine datasets with scalar subqueries
  (`SELECT (SELECT list(...) FROM a), (SELECT list(...) FROM b)`), or allow one `query` call per
  declared output dataset. Define declared datasets as the route's `output.normalized.models`.

### L2 (Low): exact-row comparison needs ORDER BY once a fixture has more than one row

- Evidence: the example SQL (`pipeline-e2e.md:84-89`) has no `ORDER BY`. That is correct for one row,
  but the guide recommends one or two records and exact child rows for fan-out (lines 9-11, 115). The
  synthetic E2E already uses `ORDER BY event.id` (`synthetic_jsonl_e2e_test.go:25`).
- Correction: add "order by a stable key when the fixture yields more than one row" to the
  Queryable output criterion.

### L3 (Low): CI timing and cache instructions sit in the contributor blueprint

- Evidence: `pipeline-e2e.md:140-145` tells contributors to "keep result caching disabled", report four
  timing components separately, and know a 10 s target on "the reference macOS ARM64 host". It is not
  stated whether that host is the local machine (3.17 s measured) or a hosted runner (CI package time
  was about 2.5x the local time: 68.1 s versus 27.7 s). How this target relates to the S3.1 budgets
  that ADR-001 rule 8 and `contributing.md:81-84` refer to is also unstated.
- Consequence: per-PR paperwork for contributors, the kind the owner wants to avoid, and an ambiguous
  target.
- Correction: move the timing and target paragraph to the proposal or the CI guide. Say that `ci/test`
  already emits setup/test seconds as JSON, so contributors only report results. Name the reference
  host and its relation to S3.1.

### L4 (Low): existing gap: edits to `replay_e2e_test.go` are never executed in CI

- Evidence: `ownership()` adds only `internal/app/aws_cloudtrail_e2e_test.go` (`ci/select.py:92`).
  `internal/app/replay_e2e_test.go` therefore falls to `shared_app` (`ci/select.py:143-145`), whose lane
  compiles `batz_app` only (`ci/test.py:170`). Its `batz_aws_cloudtrail` tag means it is vetted
  (`ci/test.py:154`) but not run.
- Consequence: the proposal's move to `batz_app` fixes this by accident. Unless it is named, any future
  pipeline-tagged file not named `<id>_e2e_test.go` recreates it.
- Correction: name it next to the `native_test.go` gap (proposal line 83-85). The runner guard plus the
  move cover it, with no extra machinery.

### L5 (Low): the shared lane has the same zero-match hole the e2e guard closes

- Evidence: the moved tests will run under `-run ^TestApp` (`ci/test.py:170`). A moved function left
  named `TestE2E...` compiles and silently never runs. The proposed guard (proposal line 93-96) covers
  only `ci/test e2e`.
- Correction: make acceptance item 2 include a one-time check that the moved test names appear as
  `pass` in the app lane's output (`go test -json` or `-list ^TestApp`). No permanent guard is needed.

### L6 (Low): prepending the empty run to the growing journey changes its literal counts

- Evidence: `replay_e2e_test.go:31-71` asserts exact acquisition and reconciliation counts (for example
  `assertAcquisition(..., 2, 0, 0, 0)` and `Observations != 15`). An empty `{"Records":[]}` file left
  in the directory changes the skipped counts on every later run. Whether `run` publishes a typed empty
  revision from an empty directory is not shown.
- Correction: state the mechanism. Either run first against an empty directory (if supported), or use
  an empty file removed before run 2. Then say the later literals stay unchanged. This is an
  implementation detail, but it is the only part of the empty-publication transfer that is unspecified.

### L7 (Low): smaller accuracy and copyability nits

- The code block uses four-space indentation (`pipeline-e2e.md:55-100`). `gofmt` will rewrite it to
  tabs. Use tabs so a copied file is already formatted.
- The baseline and example logs show `go test -json ...` (`batz-blueprint-e2e-host-baseline.log:3`).
  `ci/test e2e` runs without `-json` (`ci/test.py:186`). The overhead is negligible, but the proposal
  should name the exact command used (proposal line 22-38).
- After the move, two real shared lifecycle journeys exist: `TestAppSharedRecoveryJourney` (synthetic)
  and the CloudTrail growing journey. ADR-001's table says "one representative real shared journey"
  (line 33). Add one clause to proposal line 67 saying the second exists because identity/conflict
  reconciliation needs a source with an identity policy.

## Answers to the owner's questions

- **Minimum proof sufficient and portable?** Yes. One run, a complete published report, exact rows,
  distinguishing values, `typeof`/`epoch_ns`, a payload value and an origin tied to `parsed_revision`
  are the right floor. Optional identity is handled correctly: `source_event_id` is framework-assigned
  per observation (`replay_e2e_test.go:36-37`), so the example's origin check holds for sources with
  no logical identity. Fan-out and multi-output need only L1 and L2.
- **Example genuinely copyable?** Yes for CloudTrail. It executed unchanged apart from its name
  (`batz-blueprint-example.log`: pass, 3.17 s body, exit 0), uses only existing untagged helpers, and
  must run through `ci/test e2e`, as stated. Only L7's gofmt nit applies.
- **Removals and moves justified?** Preview, explicit-example, strict/retry and replay are moved, not
  dropped, and the retained-proof column is careful (it preserves actual sandbox, handoff, persisted
  report and retry). The empty publication is transferred, and its cost is stated honestly. Proposal
  line 67's new identity/conflict model fixtures are justified: conflict is CloudTrail SQL
  (`models/aws-cloudtrail/api_activity.sql:15`) and `test_mapping.yaml` has no conflict case. The one
  silent loss is M1.
- **Selector concrete enough?** Not yet (M2). It is concrete about helpers, vague about how CloudTrail
  model changes reach the moved tests, and silent about the cost.
- **Cache and runner fixes minimal and evidenced?** The runner guard is minimal and justified by
  `ci/test.py:182-186`. The cache path bug is evidenced, but its benefit is overstated and the example
  is invalid (M3).
- **Cost accounting honest?** Mostly. Lines 69-74 explicitly say the moves are ownership changes, not
  savings. Two gaps remain: the CloudTrail-PR shared-lane cost (M2) and the cache benefit (M3).
  Counting native starts, the before and after totals are equal (12 and 12). Only reader startups fall
  (four to one in the basic test). That matches the proposal's own modest claim.
- **Documentation consistency.** AGENTS.md, `contributing.md` §5-6 and the blueprint agree on the
  commands and the one-run shape. The relative links resolve (`docs/usage/pipeline-e2e.md` from root,
  `../adr/...` and `pipeline-e2e-proposal.md` from `docs/usage`). The only contributor-facing friction
  is L1 and L3.
- **Unnecessary machinery and vague commitments.** No new framework, registry or ledger is proposed.
  The vague commitments are M2's selection rule, M3's cache outcome and L6's empty-run mechanism.

## Disposition

Severity counts: 0 Critical, 0 Major, 3 Medium (M1, M2, M3), 7 Low (L1 to L7).

**Not ready for owner acceptance as written.** Three Mediums exceed the "more than two Medium"
threshold. All three are bounded text corrections to the proposal and blueprint. Only M1 adds anything
executable: two rows in an existing native model test, at implementation time. None changes the
blueprint's shape or asks for broader testing. One batched correction covering M1 to M3, with the Lows
fixed or explicitly declined, should make it ready for owner disposition. This review does not
authorize implementation, merge, publication or edits to the proposal documents.


---

## Parent disposition — one correction batch, 2026-11-04

The complete original report above is preserved unchanged. All three Medium and
seven Low findings retain their original severity. This is a proposal review;
implementation of the test moves, selector, runner and cache changes has not begun.
The corrected documents have not been independently re-reviewed.

Before edits the parent verified all 19 file hashes against the supplied review
manifest, with no drift. Claude's inability to run hashing commands is a disclosed
review limitation, not a hash mismatch. The parent also read Claude's complete
final response and checked each finding against current source. GitHub's current
primary documentation confirms the cache-scope and context-availability findings.

| ID | Severity | Disposition in the corrected proposal |
| --- | --- | --- |
| M1 | Medium | Requires nano-Z/nano-offset and granularity rows in the existing native mapping test before removing the old matrix. The shared journey will use a nano-with-offset A record and check exact epoch/granularity in its existing query. This preserves native reader proof without a new execution. Code changes remain implementation work. |
| M2 | Medium | Specifies a representative owner constant, asset and native-model impact triggers for `app`, explicit test-only/blueprint-only exceptions, existing representative E2E selection for shared helpers, and six selector cases. States that CloudTrail model/asset PRs gain the entire shared lane and may get slower. This tradeoff remains for owner disposition; no claim that it is approved or faster. |
| M3 | Medium | Restricts the cache benefit to same-PR reruns; resolves paths in a composite shell step via GITHUB_ENV before cache use; excludes a default-branch cache trigger. Hosted acceptance distinguishes cold new PRs from warm reruns. |
| L1 | Low | Defines route output datasets and gives scalar-subquery grouping guidance, with explicit semantic-row checks in addition to counts. |
| L2 | Low | Requires stable ordering for multi-row and fan-out results. |
| L3 | Low | Removes timing paperwork from the blueprint; points to existing command output. Names the local measurement host and preserves S3.1 budgets. |
| L4 | Low | Names the replay-file selection gap and its planned tag/name correction; warns against unowned extra pipeline-tagged files. |
| L5 | Low | Requires checking that every moved test appears as an executed pass in the one composed app run. No permanent test registry. |
| L6 | Low | Specifies a typed empty file inside the same directory, removed before adding A/B; an empty directory cannot publish typed evidence. Later observation and selection counts remain unaffected. |
| L7 | Low | Formats the snippet with gofmt, records the exact -json measurement command, and explains why an identified-source journey supplements the synthetic recovery journey. |

Validation: the original five-test host baseline passed (package 27.747 s). The
extracted proposed example passed once (test body 3.170 s, package 3.544 s, Go
command 4.675 s). The final formatted code block matches that executed example
apart from its function name and gofmt whitespace. Only prose/formatting changed
after those runs; no additional native run is warranted. Final relative-link and
whitespace checks cover the changed documents. No full suite, runtime changes,
commit or push is claimed for this proposal.

Full local command logs are retained at `/private/tmp/batz-blueprint-e2e-host-baseline.log`
and `/private/tmp/batz-blueprint-example.log`; the failed nested-sandbox attempt is
`/private/tmp/batz-blueprint-e2e-baseline.log`. The original review inputs and complete
CLI response are `/private/tmp/batz-blueprint-review-inputs.json` and
`/private/tmp/batz-blueprint-claude-response.json`.

### Reviewed and corrected document identities

- `AGENTS.md`: reviewed `ca30617f1c462145a2330d2929f2a7ca85507128273f1efbb02d4062e2fca3ad`; corrected `ca30617f1c462145a2330d2929f2a7ca85507128273f1efbb02d4062e2fca3ad`.
- `contributing.md`: reviewed `073fe8ecbcac4bb593cb8fedee5a8024a851e1afe03d8ec952a9554b7f8477e3`; corrected `073fe8ecbcac4bb593cb8fedee5a8024a851e1afe03d8ec952a9554b7f8477e3`.
- `docs/usage/pipeline-e2e.md`: reviewed `9650826ebe9ba1a8860d35ebbb0477e7afad0e407cac25dfb5e435de41b7231f`; corrected `3b2bae7048642db5e3632af4dc1896ec7345b343af6de7b7164b62513a41b079`.
- `docs/usage/pipeline-e2e-proposal.md`: reviewed `a242e0bcc56938be25470bd91bd8e714c950f34b14cfbb706ebb2c305fef0163`; corrected `b6792d7e4cfd4937bf55001d838031516d64601f51d033574a4554f0f2d281e1`.


## Subsequent owner discussion — isolate shared fixtures

The owner proposed selecting shared app behavior tests on core changes, not on
individual production pipelines. The current proposal now removes the production
CloudTrail dependency from shared tests, using isolated framework-owned fixture
content and real engines. This supersedes the M2 parent disposition that would
have selected the whole shared lane for CloudTrail model/asset changes. The above
review and hashes describe the earlier version; they are not independent review
of this revised design. No runtime or selection implementation has occurred.


## Subsequent synthetic-fixture assessment

The owner asked whether synthetic JSONL needs its own E2E job. Inspection found
that its actor/action/type/time assertions can be folded into an existing
successful synthetic run in the shared recovery journey. The proposal now calls
for that consolidation and removal of the standalone internal-probe E2E matrix
entry after replacement proof passes. Go remains the orchestration layer; real
adapter/native-model checks remain where they prove distinct boundaries. This
extension has not been independently reviewed or implemented. The 136-second
hosted job includes only 6.792 seconds inside the test package; repeated setup
and compilation dominate. These are observations from the existing CI log, not
new performance qualification.
