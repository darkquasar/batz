# S3.2-CI CI-R: Claude implementation peer review (2026-11-05)

Role: the owner's requested Claude peer review of the delivered S3.2-CI milestone (CI1 to CI5 and CI-R), against ARC-010 to ARC-013. This report is separate from the native aggregate review. It is a read-only leaf result. I edited no source, index, branch, state or progress record. This file is the only byte I wrote inside the repository. I did not delegate, install anything, run application or native suites, commit, push, clean up or use hosted execution.

Assessment: **ready for owner disposition, not approval.** I found no Critical or Major finding. There is one Medium (CIR-1), which needs either a bounded correction or explicit owner acceptance, and seven Low findings that need dispositions. Nothing here grants merge, publication or cleanup authority.

## Exact subject checked

| Item | Value |
| --- | --- |
| Root | `/Users/martinpopolito/GitHub/batz` |
| Base/HEAD | `b361019c0dd527cb156dd8697cbf153074e0b670` (`git rev-parse HEAD` matched) |
| `review-inputs.json` | `fc1cd60cc5983e63bf28605d958684e3e022eb8e7039e184336c8fdb6da1c2c3` (matched) |
| `review.patch` | `472caf65468dba7a9ec50cb28abb0db7efa94fe7238a6355d53c757e27aadc15` (matched) |
| `review-changed-files.json` | `c4115295aab6347e1577cc48d60c42d98f6ec4242ba7172f1d9474ef490f1288` (matched) |
| `delivered-check-inputs.json` | `7a5c6ca3318f9de6a462c8a6ae74e7eec56185c43b70173160e550f6180fb884` (matched) |
| Spec | `pipeline-architecture-spec.md` `01040c74a44625b87d03d86971e0f11d51c45e8c3bc139f51ddf0bd642b3b299` (matched) |
| Plan | `pipeline-architecture-plan.md` `e4ecd2de885a13a569ae1482c7632c441a49f06ab761368be8d3f87165d3e9d9` (matched) |

All 55 entries in `review-changed-files.json` matched the working tree when I read it. The three entries marked DELETED (`internal/app/aws_cloudtrail_e2e_test.go`, `replay_e2e_test.go`, `synthetic_jsonl_e2e_test.go`) are absent as declared. Comparing `initial-inputs.json` (140 entries) with `review-inputs.json` (145 entries) gives exactly the 24 task-changed source/docs files listed in `docs/reviews/2026-11-05-ci-follow-on.md`, plus four new review documents.

To isolate the task delta, I exported HEAD into `/tmp/claude-cir-review/init` (outside the repository), applied `initial.patch`, and diffed those 24 paths against the final tree (`/tmp/claude-cir-review/task.diff`: 24 files, +951/-288). `initial.patch` carries no untracked content, so for `docs/usage/pipeline-e2e.md` and `pipeline-e2e-proposal.md` (both untracked at capture) I reviewed the final bytes in full, not a delta.

Reviewed final hashes of the 24 task files (from `review-changed-files.json`, re-verified):

```text
.github/actions/prepare-macos/action.yml                2035f07317aaa747ea65787e647f38476ca0c80ff93f28b5726fa1c453e599e1
.github/workflows/pr.yml                                 3fc21a66ac9578afeafa854775bcecd7c596cde163fdac108a287d697dd2a9ec
AGENTS.md                                                a23075b04c8e7a39af76fd6a0f41e365462336c9c4475028228b44a66340591d
README.md                                                2ddd91a9e72d8d1ece99c5e3902e9c58e0bc2da101eb0f0c95299da1e8d89c69
ci/native_select_test.py                                 7b15606637a5dafe6a5f0c895a32ce3ef61279280ba5bee26ce4a044fc038142
ci/pipelines.py                                          7bc00712be8055c5c65478a069ec699804c646992a66d81e948ae6c5cdf589eb
ci/select.py                                             245bc4d1244d85b531713c3274eef879c16923de45cc29eb4fe563f1986ee56a
ci/test.py                                               d45519ca56ee83d896eb3c4caf04c4c2f6696a2fe84163d91fe6e8d9b4aff642
ci/test_entrypoints.py                                   c94c82e5e0085f26b92849abed9e2d3cfe69724998cfee2fd964e08e861eeb3a
ci/test_jobs.py                                          ada31b12fb48f730c6095dedf9ea84f85fae3119b220b74e9ffc2b84b1ce1d16
contributing.md                                          5c96b1a2cde6ebfadbdd8472a9e084d5648a998e68c65b05b8d9407d3dc9fbd5
docs/adr/0001-test-at-the-cheapest-sufficient-layer.md   898eab97261fc06b756cfc69d6d9073fb4b49940b82715aa95bd23b62ab8f1a4
docs/usage/ci.md                                         7b7e97452965eafe68b7c61b2ffc8df13833c9ec63c83e79e0362e6c164b9f23
docs/usage/pipeline-e2e-proposal.md                      a6e47c5837cd9dc5d0adbf1f9372c10f3128b989946ed855a89bd48cd2863602
docs/usage/pipeline-e2e.md                               9d4aa5d24305587690e5a42db6e61f147c4e87d4fb12f092e4bb85900546f380
internal/app/app_runtime_test.go                         5d45bd1e8243d6cb7cad3c0e5ab9aca5f767ffc400f8d1bf73ebdb3555886c84
internal/app/aws_cloudtrail_e2e_test.go                  DELETED
internal/app/publication_test.go                         ddf902377b39071cb4f2f0ca1806eec91ad374752f9d9ece59d889914aad7147
pipelines/aws-cloudtrail/fixtures/README.md              1bdbccefc0421e28e2f269cfd5d405cd8a2fe434e734da570e07c52fc8c40ce0
pipelines/aws-cloudtrail/fixtures/wiring.expected.json   3af22f79f788611ac960762d237c9723701e4c7487ebeec7bbd3eda4097fe0d9
pipelines/aws-cloudtrail/pipeline.yaml                   a5fad8e68487d620896814cec62d812bbbd445b17596bd49bb29f6d4b1f7ccb5
runtime/sqlmesh/test_models.py                           fe44fe100b0cbba04077af5a4de57f598168b5903b260e3cd23ee7d574ca4cd2
runtime/sqlmesh/test_scope.py                            6f000f1e88d36054571f1d37641aaad32181b4b79a43aeed4de38d0ff608714f
tests/aws-cloudtrail/test_mapping.yaml                   1a44a95c6a3ca2c2344a9be72a5f6ebd807ff55c4d4121d74df2babec43d8447
```

All values are copied from `review-changed-files.json` and matched the working tree.

## Delivered topology as reviewed

```text
  +--------------------+  selects   +-------------------------------+
  | PR change (select) | =========> | fast | app | models | adapters |
  +--------------------+            +-------------------------------+
            |                          |            |          |
            | pipeline-private         | core/      | native   | actual adapter
            v                          | runtime    | YAML     v
  +--------------------+               v            v    +---------------------------+
  | fast + native +    |   +----------------+  +------+  | collector => parsed      |
  | adapter contract   |   | shared journey |  |tests |  | => declared roots/audits |
  +--------------------+   | run => reader  |  +------+  | => check_build/profile/  |
                           +----------------+            |    verify_published/exp. |
                                                         +---------------------------+
                                     all jobs ~> gate (needs.*.result) [CI]
```

## Strengths (with evidence)

- **Production E2E retirement is complete and visible.** Production E2E ownership is gone from `ci/select.py:92` and from the selector return shape (`ci/select.py:221-223`). The `e2e` lane, tag helper, all-local loop, workflow matrix and gate need are all removed (`ci/test.py:231`, `.github/workflows/pr.yml:27-32,144`). `ci/test e2e` now fails argument validation with exit 2, and a test locks that in (`ci/test_entrypoints.py:15-18`). No `//go:build batz_<pipeline>` files remain: every build tag is `batz_app` (8 files). `go vet` uses `-tags batz_app` (`ci/test.py:151-154`).
- **The shared journey is synthetic-only by construction.** `nativeContent` no longer accepts caller assets and hard-codes framework-owned paths (`internal/app/publication_test.go:22`). This removes the route by which shared tests could load vendor content. The representative journey is named and required (`internal/app/app_runtime_test.go:249-251`, `ci/test.py:170`).
- **The CI4-S2 reader grouping keeps its temporal assertions.** The three recovered-catalog queries become one grouped query (`app_runtime_test.go:407`). The removed old-catalog `[[1]]` check after recovery is still covered by the held reader, which asserts `[[1,7]]` across the old and new revisions (`internal/app/shared_reader_test.go:24,28,68`). The pre-recovery old-catalog check remains at `app_runtime_test.go:346`.
- **The Go runner is stronger.** `go_tests` tracks results per package, rejects any selected skip including subtests, requires a passing top-level test in every started package, and requires the named journey (`ci/test.py:182-226`). New negative controls cover a disappeared journey and an empty second package (`ci/test_entrypoints.py:44-55`). The Python contract suite now fails on skips (`ci/test.py:168`).
- **The pipeline contract reuses production validators rather than mirroring them.** It calls `validate_contract`, one `plan_builder(...).build()` plus `apply`, `check_build`, `verify_published` (which includes `check_profile`) and the reviewed-expectation checker. It never creates a workspace, publication or reader (`runtime/sqlmesh/test_models.py:113-164`; `runtime/sqlmesh/runner.py:47-58,88-150,175-191`). `verify_published` proves exact staging membership, reject export, origin/payload linkage and mapped accounting (`runner.py:107-150`). Those were the removed E2E's payload marker and origin assertions.
- **The SEL-2 correction is sound and has a distinguishing regression.** Shared-input restatement and new-owner models are included in one applying plan (`test_models.py:132-135`). The two-owner regression goes from 2 rows to 1 row with an audit that rejects stale shared input (`runtime/sqlmesh/test_scope.py:180-226`). Distinct per-contract nonces (`test_models.py:124-125`) mean stale evidence cannot pass `check_build`.
- **The SEL-1 correction validates against the real production schema file, plus Go's `version == 1` and `repo://.` rules** (`ci/pipelines.py:59-71`, cf. `internal/app/pipeline.go:92-134`), with negative controls (`ci/test_entrypoints.py:78-93`).
- **SEL-3 and the ARC-012 selection classes are covered by focused tests.** Native-test-only edits get `fast` false (`ci/select.py:219-222`, `ci/native_select_test.py:75-81,248-251`), with a mixed control (`:283-288`). Shared SQL with no synthetic consumer excludes app (`:132-139`). A removed shared edge into the synthetic closure includes app with a reason (`:268-281`). Core deletion (`:290-294`), global config (`:296-302`), docs-only (`:91-98`) and new independent pipelines (`:141-155`) are also covered. The native-test-only exception is backed by real behaviour: the shared journey proves production ignores invalid contributor tests and README files without re-modelling (`app_runtime_test.go:411-445`).
- **CloudTrail native fixtures now pass with explicit types** (`tests/aws-cloudtrail/test_mapping.yaml:288-300,340-352`). This fixes the baseline `KeyError: caller`. The nano-offset case is now distinct by one nanosecond (`:47,69,359`).
- **Cost reporting is honest about its scope.** It states that no valid pipeline/all-local baseline exists, that the shared lane misses its budget, that no hosted qualification was done, and that start counts are source-audited only (`docs/reviews/2026-11-05-ci-follow-on.md:22-49`).
- **The evidence matches the claims.** In `delivered-global-0.log`, the Go shared app command (`:236`) has exit 0, ten top-level tests passed including `TestAppSharedPipelineJourney`, and no skips. Native selected and executed sets are 12/12 and equal (`:214`). Both actual adapter contracts printed `checks_passed` (`:200-201`). The Python contracts ran 56 tests, OK (`:190`). `delivered-core-1.log:170` and `delivered-pipeline-1.log:18` (10/10 CloudTrail native tests) are consistent. The 5-selected/3-executed line at `delivered-global-0.log:115` is inside a deliberate refusal fixture, as the brief says.

## Findings

### CIR-1 — Medium: CloudTrail's exact native `epoch_ns` proof was replaced by an unasserted probe

- **Requirement:** ARC-010 says to "Preserve exact native `typeof`/`epoch_ns`". The CI1 table's GAP row (plan line 425) says "final native epoch_ns/type/source-payload probes on the two wiring records must replace E2E values". ADR-001 rule 4 (`docs/adr/0001-test-at-the-cheapest-sufficient-layer.md:63-66`) says DataFrame equality may hide conversion loss, so explicit `typeof`/`epoch_ns` probes must be preserved where source interpretation differs. CloudTrail's offset conversion is source-specific.
- **Evidence:** `runtime/sqlmesh/test_models.py:148-151` runs `SELECT typeof("@timestamp"),epoch_ns("@timestamp")` and only prints the result (`:162-164`). Nothing compares it. The deleted E2E asserted `1792929600000000002` and `…001` exactly (task diff, `aws_cloudtrail_e2e_test.go` lines 1466-1468). `wiring.expected.json` can only assert values that are uniform across rows, and it pins `fraction_digits: 9`, which comes from the source text, not from the instant. `typeof` is still enforced by `check_profile` (`runner.py:64`), so only the epoch value is unasserted. The docs present this as evidence: "Native type/epoch queries execute on actual outputs" (`docs/usage/pipeline-e2e.md:28`, `docs/usage/ci.md:75-76`).
- **Why the native YAML tests alone do not close it:** the pinned SQLMesh harness casts the expected frame to the actual dtypes before comparing (`runtime/.local/.../sqlmesh/core/test/definition.py:280-285`). Its own comment notes that actual timestamp columns can be microsecond.
- **Failure scenario:** suppose a later edit makes staging `occurrence_time` a microsecond `TIMESTAMP` and `api_activity` casts it back to `TIMESTAMP_NS`. Then:
  - the staging native test still compares equal, because the expected `…000000002` is cast to the actual µs dtype (`test_mapping.yaml:69`);
  - the `api_activity` tests feed explicitly typed `TIMESTAMP_NS` inputs, so they pass;
  - `required_checks` compares `"@timestamp"` with staging after the cast;
  - `check_profile` sees `timestamp_ns`;
  - the wiring expectations still match;
  - and the probe prints `1792929600000000000` unasserted.

  Every lane stays green, where the deleted E2E would have failed. Confidence is source-derived (PLAUSIBLE, not reproduced). Pandas 2.3.3's `astype_overflowsafe` signature is visible in `np_datetime.pyi`, but its compiled `round_ok` default is not visible in shipped source. If that cast raises instead of truncating, SQLMesh's `errors="ignore"` keeps nanoseconds and the staging test would catch the regression. Even then, the requirement-level gap (an unasserted probe standing in for a named CI1 replacement) remains.
- **Consequence:** the one source-specific, explicit epoch-precision guarantee that the CI1 table said must replace the E2E is not enforced. Contributors and reviewers reading the contract output or docs would believe it is.
- **Bounded correction (pick one):**
  - (a) Make the representative fixture's two records express the same instant in `Z` and `+10:00` (for example `12:00:00.000000002Z` and `22:00:00.000000002+10:00`). Then add `"@timestamp": "2026-10-25T12:00:00.000000002"` to `wiring.expected.json`, so the existing uniform-value checker pins exact nanoseconds and offset interpretation at the actual-adapter boundary. This needs no format or code change, only a checksum and README update. The UTC/offset 1 ns distinction stays in the native tests.
  - (b) Have the owner explicitly accept native typed tests plus `check_profile` as sufficient, record that the CI1 GAP row is closed by them, and stop describing the probe as type/epoch evidence (or remove it).

### CIR-2 — Low: the shared-lane budget miss is returned without a concrete simplification option

- **Evidence:** the core/shared workload is 39.555 s against a 25 s tolerance (`docs/reviews/2026-11-05-ci-follow-on.md:41`). CI4-S3 shows that even ideal two-slot redistribution has a 31.663 s floor (`docs/reviews/2026-11-05-ci4-scheduling.md:47-64`). The rejected list (`:66-73`) offers no work-reducing candidate.
- **Requirement:** ARC-008 (still binding beyond its superseded E2E parts) says a miss "returns cost and a concrete simplification to the owner". ADR-001 rule 8 accepts "concrete simplification or an explicit owner decision". ARC-013 requires explicitness only.
- **Consequence:** the owner is asked to accept a 58% overrun without a costed path. Scheduling cannot close it. Only work reduction in the 40.4 s `internal/app` package can.
- **Correction:** add one costed candidate to the owner decision. Examples: split `-run` across the two native slots to approach about 31.7 s, or name which `TestApp` journey/preview executions could merge. Alternatively, record that the decision is an accepted miss. I could not check whether an earlier S3.2 handoff already carries such a candidate.

### CIR-3 — Low: the selector's "shared SQL consumed by the synthetic journey" path does not match what the journey loads

- **Evidence:** `ci/select.py:170-174` selects `app` when changed models intersect a `/testdata/` owner's scope. But `nativeContent` copies only `internal/app/testdata/synthetic`, `models/synthetic-jsonl`, `tests/synthetic-jsonl`, `config.yaml` and `runtime/sqlmesh` (`internal/app/publication_test.go:22-25`). It never copies `models/shared`, `audits`, `macros` or `seeds`. No repository `models/shared` exists today.
- **Consequence:** the ARC-012 exception is proven only on selector fixtures (`ci/native_select_test.py:268-281`). The first real synthetic dependency on repository shared or global native content will fail the shared journey at model load. That failure is visible, not a silent pass, but the docs (`docs/usage/ci.md:86-88`, `contributing.md:61-62`) describe a working path.
- **Correction:** have `nativeContent` also copy `models/shared` (and global native dirs when present). Alternatively, state that synthetic content must stay self-contained until a consumer exists.

### CIR-4 — Low: the "plus fast checks" part of CI3-A is not asserted for a new pipeline, and the walkthrough does not exercise selection

- **Evidence:** `test_independent_pipeline_addition_needs_no_registry` asserts models, adapters, no e2e and `app` false (`ci/native_select_test.py:141-155`), but not `fast`. `new-pipeline-walkthrough.py` calls only `test_models.main` in native and adapter modes (lines 27-29). It never calls `select_changes`.
- **Consequence:** CI3-A says the walkthrough "selects its own native/adapter/contracts plus fast checks and no shared application journey". Today `fast` is true only through `bool(adapters)` (`ci/select.py:222`). A later refactor could drop it unnoticed.
- **Correction:** add `self.assertTrue(jobs['fast'])` to that test.

### CIR-5 — Low: wording and link accuracy in contributor docs

- `docs/usage/pipeline-e2e.md:24` claims "exact parsed payload fidelity". The adapter check compares `json.loads` values (`runtime/sqlmesh/test_models.py:61`), not bytes.
- `README.md:28-29` lists `ci/test models aws-cloudtrail` and then `--mode adapters`. The first command already includes adapter mode (`docs/usage/pipeline-e2e.md:44`).
- The ADR status line (`docs/adr/0001-test-at-the-cheapest-sufficient-layer.md:3`) omits the 2026-11-05 amendment added at `:118`.
- The proposal header links `../../../specs/batz/...` (`docs/usage/pipeline-e2e-proposal.md:3`). That resolves only with a sibling `specs` checkout, not on a hosted render.
- **Consequence:** minor contributor confusion or overclaiming.
- **Correction:** one-line edits: say "parsed payload JSON fidelity", drop the redundant README line or annotate it, update the status line, and state that the link is to a sibling repository.

### CIR-6 — Low: before/after cost attribution is incomplete for two required PR classes and for the retired E2E

- **Evidence:** AC-ARC-013 and CI5-A require docs-only and shared-SQL classes, and an explicit attribution of the retired pipeline E2E cost. Docs-only and shared SQL are covered only by command-shape equivalence (`docs/reviews/2026-11-05-ci-follow-on.md:43`). The retired E2E is attributed structurally, with no figure (`:41`). The November 4 proposal has a single-test feasibility measurement (`docs/usage/pipeline-e2e-proposal.md:57-62`: 4.675 s Go command) and a hosted 243 s job.
- **Consequence:** the owner cannot see even a labelled, historical, non-like-for-like magnitude for what retirement removed.
- **Correction:** cite those historical numbers explicitly as non-comparable context, and state that docs-only cost is whitespace-only and unmeasured. No new measurement is needed.

### CIR-7 — Low: CI5 composed evidence is not linked from the plan

- **Evidence:** the plan's CI4 synthesis links the three CI4 reports (`pipeline-architecture-plan.md:610-613`) but not `docs/reviews/2026-11-05-ci-follow-on.md`. That file holds the composed results, dispositions and limits CI-R-A requires to be retained. The plan is frozen for this review, and the parent handoff archive is unavailable.
- **Consequence:** the traceability chain from plan to CI5 evidence depends on the coordinator's handoff.
- **Correction:** at disposition time, add one link in the plan's CI4/CI5 section, or include it in the handoff when the archive is restored.

### CIR-8 — Low (simplification): native impact is computed twice

- **Evidence:** `ci/select.py:170` and `:184` call `impacted_models(base_context,head_context,native_changes)` with identical arguments. The scopes extended between the two calls are not inputs to it.
- **Consequence:** redundant graph traversal on every native-impact selection. It is cheap today.
- **Correction:** compute it once before line 171 and reuse it.

## Requirement coverage summary

| Requirement | Result in checked scope |
| --- | --- |
| ARC-010 | Met except CIR-1 (exact CloudTrail `epoch_ns` replacement unasserted). Shared journey is synthetic-only, required and grouped; safety tests retained. |
| ARC-011 | Met: actual adapter to declared roots, audits, required checks, production profile/origin/accounting and executed expectations; explicit fixture types; no app E2E or registry. CIR-1 affects the "type/time queries" evidence quality; CIR-5 is wording. |
| ARC-012 | Met for all listed classes with focused tests. CIR-3 is a latent mismatch for the shared-SQL-into-synthetic class; CIR-4 is a missing `fast` assertion. |
| ARC-013 | Audits retained, one synthesis, bounded corrections, composed measurement with explicit limits. CIR-2 and CIR-6 cover gaps in decision inputs and attribution. |
| CI1 to CI5, CI-R | Executed in serial mode. CI4 had three settled leaves. CI5 applied one bounded batch with RED/GREEN logs. CI-R is this report plus the native review. |

## Commands and checks actually run

- `shasum -a 256` on the four `/tmp/batz-ci-follow-on` review inputs and the two specs. All matched.
- `git rev-parse HEAD` and `git status --short`.
- A read-only Python hash comparison of `review-changed-files.json` against the working tree (no mismatches), and of `initial-inputs.json` against `review-inputs.json` (the 24-file delta).
- `git archive HEAD | tar -x` into `/tmp/claude-cir-review/init`, `git apply initial.patch` there, then `diff -u` and `diffstat` for the 24 files. This was a scratch directory outside the repository; no repository state was touched.
- Read-only reads and greps of the selectors, runners, contract code, production validators, workflow and action, the shared journey and reader helper, CloudTrail models/tests/fixtures, contributor/ADR/CI docs, the CI4 reports, the evidence record, the walkthrough script, and the pinned SQLMesh `test/definition.py` and pandas metadata.
- Python JSON parsing of `delivered-results.json` and the `delivered-{global-0,core-1,pipeline-1}.log` summaries. One shell log-summary loop hung and was stopped (TaskStop). It was rerun with the Grep tool and Python parsing. No other tool was denied.

## Skipped or blocked checks

- **Not rerun, by instruction:** native, application, all-local and fast suites; the walkthrough; any experiment. That includes not confirming the CIR-1 truncation path, or the pandas `round_ok` default, by execution.
- **Not available:** hosted cache save/restore, cold/new-PR, queue, whole-job and PR critical path; a complete OS process inventory; the archive and parent handoff.
- **Not read:** `claude-review-result.json` and `claude-review-stderr.log` in the evidence directory, to keep this review independent of any earlier attempt.
- **Not reviewed:** pre-existing user changes outside the 24 task files, except where the task code calls them (`runner.py`, `inspect_preview.py`, `catalog.py`, `pipeline.go`, `explain.go`, `shared_reader_test.go`).

## Residual risks and cost/coverage limits

- The workflow gate rejects only failed or cancelled jobs (`.github/workflows/pr.yml:154-157`). A job that is selected but skipped would pass it. The CI4 selection audit noted this as hardening and did not count it, and I found no current trigger.
- Each adapter matrix job runs its contract in its own Context. Cross-owner restatement is exercised only by local batches and the new regression, which proves one ordering only.
- The shared lane is 39.555 s against a 25 s tolerance and needs an owner decision. Fast (6.088 s, within the 6.5 s owner bound), pipeline (4.276 s) and all-local (41.140 s) fit their targets locally. Fast measured 4.3 s when run concurrently after the first run, which shows warm-cache order effects. None of these figures are hosted timings.
- The build-cache key change (`action.yml:28`) is hosted-unqualified. Concurrent adapter legs share one immutable key, so only one save succeeds. That is expected and does not affect correctness, because Go's cache is content-addressed.

## Disposition request

Owner or coordinator disposition is needed for **CIR-1 (Medium)**: either the bounded correction (a) or explicit owner acceptance (b). The seven Lows need dispositions but do not block. After one bounded correction, any parent-verified fix should be recorded as parent-verified, not independently re-reviewed.

## Parent disposition — one bounded correction batch

Complete original findings above are retained. Native aggregate review completed
with 0 Critical/Major/Medium and one Low; Claude CI-R completed with 0 Critical,
0 Major, one Medium and seven Lows. Claude ran with normal manual permission
checks and authorized read/write/internet tools, exit 0, no permission denials.
Neither reviewer changed subject source. Both settled; no successor review is
admitted. Corrected bytes are parent-verified, not independently re-reviewed.

- **CIR-1 (Medium): corrected using option (a).** The existing representative
  fixture now expresses the same instant at two nanoseconds past the second in
  UTC and +10:00. The existing expectations checker compares the exact timestamp
  string `2026-10-25T12:00:00.000000002` on both actual native output rows. No DSL,
  runner, schema or new app execution was added. Its checksum and fixture notes
  agree. Source-native tests retain the distinct 1 ns/2 ns UTC/offset cases and
  complete final time/origin projections. Adding the timestamp expectation before
  changing the fixture observed intended RED (`ci/test models aws-cloudtrail
  --mode adapters`, exit 4, `reviewed value expectation failed: @timestamp`).
  Corrected GREEN is exit 0 and logs two TIMESTAMP_NS epochs
  `1792929600000000002`, with production required/profile/accounting/origin checks.
  Logs: `/tmp/batz-ci-follow-on/cir-epoch-{red,green}.log`.
- **Native C-L3 (Low), overlapping CIR-5: corrected.** README identifies which
  invocations build a collector and splits native/adapters commands without
  duplicate adapter execution. The CI guide already had the correct exception.
- **CIR-2 (Low): decision input added.** A concrete next simplification candidate
  is to carry explicit route/example and fractional metadata assertions from
  `TestAppExplicitLocalExampleWithRouteIsAccepted` in one of the existing two
  native preview executions. That separate preview costs about 2.4 s in the
  final core log; removing one collector/discovery/model/inspection cycle would
  project roughly 37.2 s from 39.555 s, still above 25 s. This is a source-backed
  estimate, not a measured saving. Preserve the distinct repo/workspace resolution
  proof at its cheaper boundary before any future merge. The candidate is not
  implemented in this bounded CI-R correction, and budget acceptance remains an
  explicit owner decision. Two-slot redistribution alone also cannot meet 25 s.
- **CIR-3 (Low): current constraint documented.** Repository synthetic models are
  currently self-contained; the shared journey constructs its own invented shared
  ancestor. A future real shared/global consumer also requires its consumed content
  in the fixture-copy helper. Documentation now distinguishes graph selection
  from copying. No repository shared/global dependency exists or is claimed to
  load automatically today; the native base/head selector exception remains tested.
- **CIR-4 (Low): corrected.** The independent new-pipeline selector case now
  explicitly asserts `fast`, alongside its native/adapter selection and no app.
  The temporary native/adapter walkthrough and the separate real-Git selector
  control are complementary evidence, not a claim that the walkthrough invoked
  selection. All 26 selector tests pass after this assertion.
- **CIR-5 (Low): corrected.** Payload wording now promises JSON fidelity; README
  removes duplicate checks; ADR status includes November 5; the proposal marks its
  architecture-plan link as a local sibling-checkout link.
- **CIR-6 (Low): attribution clarified.** Docs-only selects whitespace checking;
  its standalone complete PR cost is unmeasured. Shared-SQL classes reuse matching
  selected command shapes, not separately measured walls. Historical November 4
  context: the one-test blueprint feasibility command took 4.675 s; the earlier
  hosted CloudTrail E2E job took 243 s. They concern different work/topology and
  are neither a valid current before baseline nor subtraction-based savings.
  Retiring the pipeline app boundary removes that job's app/discovery/publication/
  reader responsibility structurally; the preserved core journeys still cost work.
- **CIR-7 (Low): plan link added at disposition time.** The existing plan links
  the composed evidence and complete native/Claude reports. The archive handoff
  still awaits normal authenticated restoration; no partial archive was edited.
- **CIR-8 (Low): corrected.** The same `impact` set is reused after scope expansion;
  model-impact arguments did not change. Existing green graph controls cover the
  behavior-preserving removal of the duplicate traversal.

Parent verification after the batch: actual adapter contract GREEN exit 0;
`python -I -m unittest discover -s ci -p native_select_test.py` exit 0 (26 tests);
`ci/test fast` exit 0 (6.009 s, all 14 CI wrapper/schema/job tests, 8 preview
contracts, Go tests and tagged vet). Full logs are
`/tmp/batz-ci-follow-on/cir-{epoch,selection,fast}-green.log`. Only those affected
checks were repeated. App/runtime/native model SQL and the 12 native YAML test
bodies are unchanged from the passing aggregate; source fixture/expectations and
selector corrections have fresh focused proof. The earlier all-local 41.140 s
and core 39.555 s observations bind the reviewed pre-correction inputs and are
not new post-correction measurements. Prose/link/whitespace checks cover later
report bookkeeping without another runtime suite.

Exact old/new per-file hashes: `/tmp/batz-ci-follow-on/review-correction-delta.json`.
Corrected source inventory: `/tmp/batz-ci-follow-on/correction-inputs.json`.
No Critical/Major/Medium code finding remains unresolved. The original CIR-1
Medium label is preserved above; its correction is not a severity downgrade.
Shared-budget acceptance, hosted/platform qualification and full observed OS
process inventory remain explicit limits. No commit, push, hosted run, source
cleanup or archive bypass occurred.
