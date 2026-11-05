Ready for owner disposition: **no Critical, Major or Medium findings; one Low documentation residual**. The shared timing miss and unavailable hosted qualification still require explicit owner disposition. This native review does not replace the separately requested Claude CI-R review.

```text
+------------------+  selected proof  +-----------------------+
| Pipeline content | ==============> | Native + adapter      |
+------------------+                  +-----------------------+

+------------------+  selected proof  +-----------------------+
| Core behavior    | ==============> | Synthetic app + safety|
+------------------+                  +-----------------------+
```

**C-L3 — Low, retained documentation finding: README still overstates collector builds.**

- Location: `README.md:35`.
- Evidence: “Each native invocation builds its own collector and dependency receipt” conflicts with `ci/test.py:248`, which deliberately bypasses `prepare_native()` for `models --mode native`. The corrected CI guide describes this exception at `docs/usage/ci.md:35`.
- Consequence: contributors receive inconsistent preparation and cost information. Runtime behavior is correct.
- Bounded correction: describe app and adapter/combined-model invocations as building the collector, and state that native-only model tests do not.
- Original identity/severity retained: **C-L3, Low**. Its original CI-guide location is corrected; the same overstatement remains in README.

No other defect was established in the checked scope.

The delivered changes have these supported strengths:

- `internal/app/publication_test.go:13` restricts shared fixture copying to invented synthetic assets. `internal/app/app_runtime_test.go:245` composes the representative run-to-reader/recovery journey, and `:403` retains exact native epochs, payload and parsed-revision origin assertions.
- `runtime/sqlmesh/test_models.py:119` calls production execution-contract validation; `:138` validates build identity/accounting and required checks; `:147` invokes production profile/origin/payload membership validation. This path creates no app workspace, publication or reader.
- `runtime/sqlmesh/test_models.py:132` uses one plan-builder/build/apply operation for overlapping shared refresh and newly selected models. The distinguishing two-owner regression is at `runtime/sqlmesh/test_scope.py:180`.
- `runtime/sqlmesh/test_models.py:89` compares selected identities with executed successes, count, failures and skips. `ci/test.py:182` additionally checks Go package results and requires the representative shared journey.
- `ci/select.py:170` retains base/head consumer impact before test-scope expansion; `:219` suppresses fast for attributable native-YAML-only changes while mixed/global/core changes retain broader checks.
- `tests/aws-cloudtrail/test_mapping.yaml:287` retains complete final conflict/origin structures. `:339` carries distinct 1 ns/2 ns timestamps and complete final time/origin structures. The actual adapter log also shows the distinct integer epochs.
- `.github/workflows/pr.yml:98` disables unused native-only build-cache transfer. `.github/actions/prepare-macos/action.yml:28` removes pipeline identity from the identical collector compilation cache key while preserving job/platform/module partitioning.
- `.github/workflows/pr.yml:143` requires successful selection and rejects failed/cancelled jobs. Requested-work omissions/skips are checked inside the runners.

The raw epoch probes at `test_models.py:151` execute and are logged rather than directly compared there. I investigated this as a possible gap, but did not retain a finding: ARC-011 explicitly requires probe execution; production validation asserts the output’s `TIMESTAMP_NS` type; source-native fixtures independently retain the distinct timestamp expectations. A default DataFrame-conversion risk alone does not establish an uncovered current failure.

**CI4 finding disposition checked against final source and retained evidence:**

| Source ID and original label | Checked disposition |
| --- | --- |
| SEL-1 — Medium | Corrected: production schema/version/project validation at `ci/pipelines.py:59`, called before both runner modes at `test_models.py:168`; positive and four mutation controls at `ci/test_entrypoints.py:78`. Retained RED shows three intended failures; GREEN passes. |
| SEL-2 — Medium | Corrected: combined refresh/new-owner plan at `test_models.py:132`; actual-adapter shared-ancestor audit regression at `test_scope.py:180`. Retained RED fails the second owner’s application; GREEN passes the 2-row to 1-row transition. |
| SEL-3 — Medium | Corrected: native-test-only fast suppression at `ci/select.py:219`; production/synthetic and mixed-change selector assertions retained. RED has the two intended fast-selection failures; GREEN passes. |
| C-L1 / CI4-S1 — Low, duplicate consequence | Corrected at `pr.yml:98`. |
| C-L2 — Low | Corrected at `action.yml:28`; hosted benefit remains unqualified. |
| C-L3 — Low | CI guide corrected; README residual reported above. |
| CI4-S2 — Low | Corrected: grouped recovered-catalog query at `app_runtime_test.go:407`; held-reader old/new comparison remains at `shared_reader_test.go:68`. The distinct post-failure old-catalog query remains. |
| CI4-S3 — evidence limitation | Preserved accurately: scheduling does not establish a passing shared budget. |

The CI4 synthesis preserves rejected alternatives and their reasons. I found no hidden implementation of extra shards, binary caching, broad cache prefixes, default-branch triggers or another orchestration framework.

**Exact reviewed subject and identity checks:**

- Root: `/Users/martinpopolito/GitHub/batz`.
- Base/HEAD: `b361019c0dd527cb156dd8697cbf153074e0b670`, plus captured dirty/untracked content.
- Initial capture and patch distinguish existing cleanup from this milestone; no prior user changes were discarded.
- Confirmed these supplied SHA-256 identities:

```text
review-inputs.json
fc1cd60cc5983e63bf28605d958684e3e022eb8e7039e184336c8fdb6da1c2c3
review.patch
472caf65468dba7a9ec50cb28abb0db7efa94fe7238a6355d53c757e27aadc15
review-changed-files.json
c4115295aab6347e1577cc48d60c42d98f6ec4242ba7172f1d9474ef490f1288
delivered-check-inputs.json
7a5c6ca3318f9de6a462c8a6ae74e7eec56185c43b70173160e550f6180fb884
pipeline-architecture-spec.md
01040c74a44625b87d03d86971e0f11d51c45e8c3bc139f51ddf0bd642b3b299
pipeline-architecture-plan.md
e4ecd2de885a13a569ae1482c7632c441a49f06ab761368be8d3f87165d3e9d9
```

All **145 review-input entries** and **138 delivered-check entries** matched the worktree before and after inspection. The hash-bound inventories retain the complete exact file hashes, including supporting source and review documents. The 24 milestone files independently checked were:

```text
.github/actions/prepare-macos/action.yml
2035f07317aaa747ea65787e647f38476ca0c80ff93f28b5726fa1c453e599e1
.github/workflows/pr.yml
3fc21a66ac9578afeafa854775bcecd7c596cde163fdac108a287d697dd2a9ec
AGENTS.md
a23075b04c8e7a39af76fd6a0f41e365462336c9c4475028228b44a66340591d
README.md
2ddd91a9e72d8d1ece99c5e3902e9c58e0bc2da101eb0f0c95299da1e8d89c69
ci/native_select_test.py
7b15606637a5dafe6a5f0c895a32ce3ef61279280ba5bee26ce4a044fc038142
ci/pipelines.py
7bc00712be8055c5c65478a069ec699804c646992a66d81e948ae6c5cdf589eb
ci/select.py
245bc4d1244d85b531713c3274eef879c16923de45cc29eb4fe563f1986ee56a
ci/test.py
d45519ca56ee83d896eb3c4caf04c4c2f6696a2fe84163d91fe6e8d9b4aff642
ci/test_entrypoints.py
c94c82e5e0085f26b92849abed9e2d3cfe69724998cfee2fd964e08e861eeb3a
ci/test_jobs.py
ada31b12fb48f730c6095dedf9ea84f85fae3119b220b74e9ffc2b84b1ce1d16
contributing.md
5c96b1a2cde6ebfadbdd8472a9e084d5648a998e68c65b05b8d9407d3dc9fbd5
docs/adr/0001-test-at-the-cheapest-sufficient-layer.md
898eab97261fc06b756cfc69d6d9073fb4b49940b82715aa95bd23b62ab8f1a4
docs/usage/ci.md
7b7e97452965eafe68b7c61b2ffc8df13833c9ec63c83e79e0362e6c164b9f23
docs/usage/pipeline-e2e-proposal.md
a6e47c5837cd9dc5d0adbf1f9372c10f3128b989946ed855a89bd48cd2863602
docs/usage/pipeline-e2e.md
9d4aa5d24305587690e5a42db6e61f147c4e87d4fb12f092e4bb85900546f380
internal/app/app_runtime_test.go
5d45bd1e8243d6cb7cad3c0e5ab9aca5f767ffc400f8d1bf73ebdb3555886c84
internal/app/aws_cloudtrail_e2e_test.go
DELETED
internal/app/publication_test.go
ddf902377b39071cb4f2f0ca1806eec91ad374752f9d9ece59d889914aad7147
pipelines/aws-cloudtrail/fixtures/README.md
1bdbccefc0421e28e2f269cfd5d405cd8a2fe434e734da570e07c52fc8c40ce0
pipelines/aws-cloudtrail/fixtures/wiring.expected.json
3af22f79f788611ac960762d237c9723701e4c7487ebeec7bbd3eda4097fe0d9
pipelines/aws-cloudtrail/pipeline.yaml
a5fad8e68487d620896814cec62d812bbbd445b17596bd49bb29f6d4b1f7ccb5
runtime/sqlmesh/test_models.py
fe44fe100b0cbba04077af5a4de57f598168b5903b260e3cd23ee7d574ca4cd2
runtime/sqlmesh/test_scope.py
6f000f1e88d36054571f1d37641aaad32181b4b79a43aeed4de38d0ff608714f
tests/aws-cloudtrail/test_mapping.yaml
1a44a95c6a3ca2c2344a9be72a5f6ebd807ff55c4d4121d74df2babec43d8447
```

**Actual review checks and execution evidence:**

I read ARC-010–013, CI1–CI-R, the CI1 replacement table, execution-mode/synthesis amendments, repository instructions, contributor guide, ADR, changed source/docs, relevant production validators and callers, complete CI4 reports, retained RED/GREEN logs and final execution logs. Commands were read-only `cat`, `sed`, `nl`, `rg`, `git rev-parse`, `git status`, `git diff`, and Python hashing/log/AST/link inspection.

- `git diff --check`: exit 0.
- AST parsing of all 10 changed Python files: passed without compilation or writes.
- Relative-link inspection across captured changed Markdown: no missing local targets.
- Final inventory comparison: no drift.
- One mistyped log-read path returned exit 1; its corrected read succeeded. It was not a verification failure.

I **did not rerun native/application suites**, install anything, invoke CLI models, delegate or mutate source/index/state/report files.

The supplied final evidence shows:

| Workload | Actual commands | Exit | Wall seconds |
| --- | --- | ---: | ---: |
| Fast | `ci/test fast` | 0 | 6.088 |
| Pipeline | Concurrent `ci/test fast` and `ci/test models aws-cloudtrail` | 0 | 4.276 |
| Core | Concurrent `ci/test fast` and `ci/test app` | 0 | 39.555 |
| Global | `ci/test all-local` | 0 | 41.140 |

I checked `delivered-results.json` against the referenced `delivered-*.log` command/result records. All-local includes the 12 selected/executed native identities, both actual adapter contracts, 56 Python shared contracts, shared app/run/scratch checks, fast Go checks and tagged vet. Internal canned failures are expected refusal-test inputs; enclosing suites pass.

The independent native-mode evidence reports 12 executed tests and exit 0. The temporary `example-jsonl` walkthrough independently passes native and actual-adapter modes using manifest/models/native YAML/fixtures, without an app test or registry entry.

Acceptance limits remain material and accurately disclosed:

- Core/shared **39.555 s exceeds the 25 s tolerated bound**. This review does not accept that miss for the owner.
- Pipeline/all-local have no valid passing before baseline, so no numerical before/after savings are established for them.
- Shared-SQL command-shape equivalence is reasoned from selection, not another measured wall time.
- Native/process counts are source-audited estimates corroborated where logs expose them; they are not a complete observed OS-process inventory. The interrupted cancellation Context remains uncertain.
- No matching post-change hosted save/restore, cold/new-PR, queue/job/PR or remote transport qualification exists.
- The archive authentication failure justifies the approved `docs/reviews/` fallback; partial archive/index and parent handoff remain untouched.
- The gate does not separately compare every selected flag with successful job status. As in CI4, I found no present normal-workflow trigger establishing an additional concrete skip defect.

Assessment: **native aggregate review complete; ready for owner disposition of the retained Low item and explicit acceptance limits**. No merge, publication, milestone completion or successor-review authority is inferred. No background work or descendants remain.

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
