# Proposal: make the pipeline E2E a small contributor blueprint

Historical November 4 proposal. S3.2-CI CI1 in the [current architecture plan (local sibling specs checkout)](../../../specs/batz/01-pipeline-architecture/pipeline-architecture-plan.md#ci1--compare-proof-before-deleting-pipeline-e2e) supersedes its replacement table and per-source Go E2E requirement. Current guidance is [contributing](../../contributing.md). Prior measurements/reviews below remain historical evidence.

Status: owner-approved implementation amendment to the CI simplification work in
`../specs/batz` (S3.1/S3.2), 2026-11-04. No S5-S9 implementation is included.

Execution mode: serial. The fixture/helper and coverage moves share the same
application journeys (sections below); one owner composes them before verification.
Strategy: green coverage for fixture/test refactoring, test-first selection and
runner fixes, focused postchecks for prose and cache wiring. No runtime safety
invariant changes. Base/head: `b361019c0dd527cb156dd8697cbf153074e0b670` plus working changes.

[Claude's complete review and parent dispositions](../reviews/2026-11-04-pipeline-e2e.md)
are retained. The original review found three Medium and seven Low issues; the
proposal was corrected once afterward. The subsequent owner discussion changes
the design further: shared app tests use framework-owned fixtures, rather than
production CloudTrail content. This supersedes the representative-owner trigger
proposed in response to finding M2. The [fresh design review](../reviews/2026-11-04-pipeline-e2e-design.md)
found two Medium and six Low issues; their bounded dispositions are retained below.
The earlier report remains historical evidence.

## Measured problem

The [successful PR run](https://github.com/darkquasar/batz/actions/runs/41790344794/job/126442454553)
at `b361019c0dd527cb156dd8697cbf153074e0b670` took 243 seconds for CloudTrail E2E.
The job reported 51.099 seconds dependency preparation, 69.835 seconds collector
build, 108.421 seconds for the Go test command (68.110 seconds inside the test
package), plus checkout/cache/job overhead. The preceding run took 224 seconds
with 56.513 seconds inside the package. Whole-job time is the contributor's wait;
test execution alone does not explain it.

Both runs warn that cache save patterns containing `..` are invalid. The workflow
defines caches under `${{ github.workspace }}/../batz-cache/...`; the cache action
rejects those patterns. Fixing the paths is necessary to make cache saves work.
It does not guarantee a warm hit or eliminate dependency installation on fresh VMs.
This workflow runs only on pull requests. Its saved caches are scoped to the PR's
merge ref, so the path fix enables reuse on subsequent runs of that same PR, not
the first run of another PR. Default-branch cache population would be a separate
decision; this proposal does not add a new trigger or promise cross-PR warm builds.
See [GitHub's cache scope rules](https://docs.github.com/en/actions/reference/workflows-and-actions/dependency-caching#restrictions-for-accessing-a-cache).

A focused local host baseline at the same source revision executed five E2E tests
in 27.747 seconds (Go command 28.780 seconds, collector build 2.620 seconds):

The measurement used `go test -json -tags batz_aws_cloudtrail ./internal/app -run
'^TestE2E' -p 1 -count=1` under the existing `ci/test.py` environment and private
runtime preparation. `-json` adds per-test timing to the normal lane's command.

| Existing test suffix | Execution | Current scope |
| --- | --- | --- |
| `NativePreviewComposesWorkspaceSelectionStructuresMacrosAndRepeatability` | 5.90 s | Two native previews, workspace/config/macros/structure/repeatability and inspection failures. |
| `ExplicitLocalExampleWithRouteIsAccepted` | 2.55 s | Another native preview for explicit sample association and fractional-time metadata. |
| `TinyMixedNativeWiringRetainsStrictPartial` | 3.29 s | Strict partial run, handoff environment, durable report and model-reuse retry. |
| `LocalFixturePublishesQueryableRevision` | 4.84 s | Empty run, then seven-record publication with history/time/selection checks. |
| `GrowingDirectorySkipsChangesAndReplaysWithoutLosingHistory` | 10.87 s | Five run invocations for new/changed/replayed/unchanged evidence and conflict reconciliation. |

The blueprint's Go code block was extracted into a temporary test with a unique
name and executed once through the same installed native tools. It passed:
3.170 seconds for the test body, 3.544 seconds for the package and 4.675 seconds
for the Go command. The temporary test was removed after execution; no permanent
test was changed. This is a local feasibility measurement, not a hosted CI result
or proof that the proposed shared-test moves are complete.

An initial tool-sandbox attempt could not launch the OS sandbox for preview; the
host run passed all five tests with normal permissions. Neither run was all-local.

## Minimum proof for any pipeline

Adopt the [blueprint](pipeline-e2e.md) as the contributor-facing specification:
one named route and small fixture through actual acquisition, normalization,
publication and an ordinary read-only query. Assert expected complete publication,
exact rows, distinguishing semantics, time representation and sample payload/origin
references. A source without logical identity remains valid. Fan-out/multi-output
pipelines check all expected output datasets in the same run. New transports need
their own actual acquisition proof; this is not blanket support qualification.

The blueprint links directly to the executed `aws_cloudtrail_e2e_test.go`.
There is one maintained template, with no duplicate prose code block.
Keep the passing criteria in the guide. No new framework or fixture DSL.

## Separate framework proof from pipeline proof

The earlier proposal merely retagged tests while retaining CloudTrail as their
input. That preserved a dependency on CloudTrail's models and forced CloudTrail
changes to select the shared lane. The revised proposal removes that dependency.

Use the existing internal synthetic pipeline as the starting point for shared
preview, strictness and lifecycle fixtures. These tests still execute real Bento,
SQLMesh, storage, publication and reader code. Invented source data does not mean
mocking the framework. Their isolated project must contain only framework-owned
fixture models/configuration: changing a production pipeline must not change what
these tests load or execute. The former `nativeContent` helper copied the whole catalog. It now defaults
to only internal synthetic content; pipeline E2Es pass their explicit owned paths. Reuse
existing fixture/copy helpers to construct the small project; no fixture registry
or new test framework.

| Current coverage | Revised destination and required proof |
| --- | --- |
| Basic CloudTrail source-to-reader journey | The one-run blueprint, using the real CloudTrail manifest, adapter and models. |
| Preview composition, repeatability, explicit sample and sandbox behavior | Shared `batz_app` / `TestApp...` tests using internal fixture content and their real preview execution. Source-specific preview metadata stays with its source at the cheapest sufficient layer. |
| Strict partial behavior and retry | Shared tests with invented valid, rejected and unsupported records. Retain actual handoff, durable reports, publication and model-reuse proof; policy tests alone are insufficient. |
| Growing directory, changed files, replay, no-op and immutable history | Shared journey using internal fixture content. Assert generic acquisition and origin membership independently of CloudTrail identity rules. Combine with existing synthetic recovery coverage where that preserves distinct transitions and reduces native starts. |
| CloudTrail logical identity, duplicate combination and conflict flags | Native CloudTrail model tests with explicit input columns, reliable/absent identity, repeated payloads and conflicting variants. Generic multi-origin integrity stays in bridge positive/negative tests; direct shared real-DuckDB tests now cover framework reconciliation accounting (including absent identity fields). |
| Empty publication followed by new membership | Retain once in the shared native journey. Use a supported typed empty input inside the same directory and remove it before adding A/B. An empty directory cannot supply a typed publication. Recalculate fixture expectations for its framing instead of copying CloudTrail counts. |
| Source time interpretation | Add nano-Z/nano-offset and `fraction_digits` expectations to CloudTrail's existing native mapping test before removing its larger matrix. Keep exact native epoch probes where needed; dataframe equality alone must not hide truncation. |
| Framework time preservation through replay/publication | Keep nano-distinct, coarse, missing-time and historical observations in the internal shared journey, with grouped native `epoch_ns`/type queries; source offset/granularity interpretation belongs to CloudTrail native tests. No per-pipeline repetition of that matrix. |

Before deleting a CloudTrail assertion, identify its replacement and observe it
pass at the required boundary. Replacement native/shared checks passed before the old assertions were removed. This is a fixture/coverage refactor, not a decision to drop safety or
precision promises.

The intended saving is that pipeline-only PRs do not run unrelated framework
journeys, and future contributors add one small E2E rather than copying five.
Shared work still costs time on core changes. Consolidation and fewer reader
starts may reduce total work, but must be measured; the 3.170-second blueprint
measurement is not a measurement of the revised shared suite. Cache path repair
separately enables caching for same-PR reruns.

## Synthetic JSONL: reuse the shared native journey

The successful hosted synthetic E2E job took 136 seconds. Its log reports 34.434
seconds dependency preparation, 53.873 seconds collector build, and 37.236 seconds
for Go test, of which 6.792 seconds executed inside the test package. Roughly 30
seconds was Go test compilation/startup; the remaining job time was overhead.
The long job is mainly repeated preparation on a separate VM, not an expensive
JSONL assertion loop.

The standalone test makes one real run and checks two mapped actors/actions,
TIMESTAMP_NS and exact nanosecond/millisecond values. This is shared framework
integration proof using invented data, not a supported vendor integration. JSONL
framing/fidelity remains covered separately by the actual adapter checks; mapping
examples remain in native model tests.

Fold the distinguishing output assertions into an existing successful execution
in `TestAppSharedRecoveryJourney`, then remove the redundant standalone test and
its E2E matrix entry. For example, that journey already runs an `other` binding
before its later recovery steps: feed it the original two-row JSONL fixture and
replace its later count-only query with the ordered actor/action/type/epoch query.
Update its expected count to two while preserving the independent binding and
old-catalog checks. The first binding's one-row initial state and later seven-row
recovery remain unchanged. Group origin/report assertions into the same existing
results. This adds neither a run nor a model context. Check that any source/model
mutations in the shared journey preserve the original assertions' meaning before
removing the standalone test.

Keep Go as the orchestration language: it directly calls the production Go entry
point and uses ordinary DuckDB for reader proof. Rewriting in Python would not
remove collector compilation or SQLMesh startup and would require another way to
invoke the application. Reuse executions and prepared runtime within the shared
job instead. Preserve the real engines and exact time assertions.

Use existing manifest classification (`/testdata/`) to keep internal probes out
of the pipeline E2E matrix and all-local's per-pipeline E2E loop. Changed internal
fixture assets or models select the shared app lane; production pipelines retain
their minimal E2Es. Keep internal adapter/native-model checks where they provide
distinct lower-layer proof. Add a focused selection check that synthetic fixture
changes run shared tests without spawning a separate synthetic E2E job. No new
pipeline registry or custom test framework.

Removing the separate job avoids its repeated VM preparation/build for this proof.
It does not guarantee the PR finishes sooner if another parallel job determines
the critical path. Measure total work and the shared job's time after folding.

## Selection and runner changes needed with the moves

- Put shared functions in shared files with `batz_app` and `TestApp` names. Merely
  renaming the source file does not change which Go tests run.
- Keep source-specific mapping/identity assertions in native model fixtures.
  An ordinary new pipeline's isolated changes select its adapter/models/minimal E2E.
  Shared preview/lifecycle changes select shared tests once. Updates to helpers
  such as `internal/app/native_test.go` must select affected E2E too: currently
  they only select the shared app lane. Add a focused selector regression for this
  existing gap, using explicit shared helper paths rather than a new registry.
  Select the existing `broaden` representative set (first production pipeline only, plus shared `app` coverage; internal probes
  never enter E2E selection), not every pipeline. Also cover the current gap where
  editing `replay_e2e_test.go` alone selects `app` but its CloudTrail tag prevents
  execution. Moving it to `batz_app` fixes that case; keep future pipeline E2Es in
  their canonical `<pipeline>_e2e_test.go` file unless ownership is explicitly added.
- Shared tests depend only on core application/runtime code, shared test helpers,
  internal fixture content, relevant shared configuration and dependency/CI
  changes. Those inputs select the shared job. Production pipeline-only changes
  select that pipeline's model/adapter/minimal E2E checks, not the shared job.
  No CloudTrail representative-owner exception is needed after fixture isolation.
  Changes to integration boundaries may additionally select existing representative
  minimal E2Es, rather than every pipeline. Shared model changes select their
  actual consumers; they are not treated as isolated pipeline changes.
- Add focused selector cases: CloudTrail model-only and Workspace-only additions
  do not select `app`; internal fixture changes and core retry changes do; shared
  helpers select their representative E2Es; native-test-only and canonical E2E-only
  edits retain narrow selection. Verify fixture isolation by inspecting copied
  content and ensuring shared fixture execution does not read production source
  assets. Do not merely suppress the shared job while leaving the dependency.
- Have `ci/test e2e` use Go's JSON output to require an executed, passing matching
  test and reject skipped selected tests. Today a correctly tagged file can
  contain no matching `TestE2E` functions and still exit successfully. Reuse
  standard Go output; no assertion framework or manifest test registry.
- Resolve tool/cache paths in an initial shell step of the existing preparation
  composite action: write absolute `$RUNNER_TEMP/batz-*` paths to `$GITHUB_ENV`
  before the cache steps, replacing the workflow-level `../` definitions. Set
  all related tool/cache variables consistently for preparation and later tests.
  The `runner` context is not available in workflow-level `env`; do not put
  `${{ runner.temp }}` there. Keep generated files outside the checkout so fast
  whitespace scanning does not traverse them. Retain pins and fresh source builds.
  See [context availability](https://docs.github.com/en/actions/reference/workflows-and-actions/contexts#context-availability).

## Acceptance of the cleanup

1. The CloudTrail E2E file is the small, executed blueprint; its job runs that
   journey only. Fold synthetic output/report/origin assertions into an existing
   shared native execution, then remove its standalone E2E job without adding
   another acquisition or model execution.
2. Replacement shared/native assertions above remain selected and pass before
   removing the old source-coupled tests. Shared fixture projects do not load
   production pipeline assets. Demonstrate
   zero-test and skipped-test refusal with cheap runner checks. Test-only model
   fixtures continue to pass in standalone native mode. In the one composed run,
   capture `-json` output for the app lane and check every moved function appears
   as an executed pass; no permanent shared-test registry/guard is required.
3. Run the affected model, E2E, shared and selector checks once on final changes.
   Measure composition once when all moves are integrated. Report total work as
   well as per-job times; do not hide moved cost.
4. Target at most 10 seconds warm test-body execution for the one-run blueprint
   on the local macOS ARM64 development host used for the 3.170-second observation,
   not a GitHub runner. This additional per-blueprint target does not replace the
   existing S3.1 workload budgets or their required composition measurements.
   This is an initial proposal, to validate, not a
   new wall-clock assertion or a guarantee for cold hosted jobs. Record native
   starts and actual time rather than only increasing a timeout.
5. Hosted evidence shows cache save warnings gone and whether a subsequent run
   of the same PR restores its relevant build cache. A first run on a new PR stays
   a separate cold observation; cross-PR warming is not included. Publish the
   actual whole-job and PR-critical-path change;
   no promised four-minute-to-ten-second CI reduction before measurement.

Contributor and root agent guidance now link to the blueprint and remove the
blanket all-local requirement for an isolated pipeline contribution. The test moves, runner guard and cache fix are implemented under owner direction.
Verification and review results below bound the claims; hosted cache/wait evidence
remains a separate qualification. No acceptance promise is retired.

## Fresh design review: bounded correction dispositions

- M1: preserve framework `reconciliation_counts` in the existing real-DuckDB
  Python contracts with invented duplicate/conflicting/unidentified/excluded rows
  and absent identity columns. CloudTrail grouping rules remain source-owned.
- M2: `/testdata/` ownership reached through assets, model impact or native tests
  selects `app`; probes never select E2E. Broadening/helpers select the first
  production E2E and `app`. All-local excludes internal probes from its E2E loop.
- L1: owner clarification includes untagged fast tests. Shared roots copy only
  invented fixture assets/models/tests plus shared configuration/runtime. Scoped
  production E2E roots explicitly opt into their own assets. Replace direct
  repository reads and foreign-model examples with internal invented content.
- L2: the invented fixture gains a minimal `disposition` rule for rejected and
  unsupported rows, with native tests; the collector rejects non-object records.
- L3: shared time probes preserve native nano/coarse/missing/historical values.
  They do not duplicate vendor offset or granularity interpretation.
- L4: E2E JSON events require a top-level executed matching pass and a passing
  package; reject all selected skips, including subtests. Preserve readable Output.
- L5: before measurements captured before source edits: fast 6.145 s, shared app
  lane 22.052 s, CloudTrail+synthetic E2E lane 36.348 s, models/adapters 5.083 s.
  Logs and initial hashes are `/tmp/batz-before-*-host.log` and
  `/tmp/batz-simplification-before-inputs.json`. Collector builds are separate
  setup work. Exact build-cache keys remain frozen on a hit; later commits may
  compile against that older cache. Hosted costs/cache reuse require hosted evidence.
- L6: final contributor guidance describes the implemented state. An empty typed
  file is removed before A/B; its committed zero-row revision remains history,
  while removed files do not acquire new membership or count as skipped files.

Parent checked the applicable peer S3.1/S3.2 cost and verification sections (the
reviewer lacked peer-repository permission). Replacement assertions must pass
before retiring source-coupled assertions. No safety/precision/lineage promise
is retired.

## Implementation verification and measured limits

Implemented serially at the base/head above, without a commit. The maintained
example is now the canonical CloudTrail Go E2E (49 lines). Shared preview,
strictness, file tracking/replay, recovery and fast preflight/explain tests use
only the invented fixture. The original two-row JSONL output assertions run in
the recovery journey's existing `other` binding. No acquisition/model execution
was added for that consolidation. Its standalone test and selector-generated CI
job are removed. No framework, registry, fixture cache or coverage ledger was added.

Verification evidence is in `/tmp/batz-*.log`:

- Existing baseline `ci/test fast`, `ci/test app`, `ci/test e2e aws-cloudtrail
  synthetic-jsonl` and `ci/test models aws-cloudtrail synthetic-jsonl` all passed
  on the host before source edits. The first sandboxed app attempt could not use
  `/bin/ps` or the OS sandbox and failed; it is not timing/qualification evidence.
- Runner and selector public-interface regressions failed for the intended old
  behavior, then passed (`batz-runner-{red,green}.log`,
  `batz-selector-{red,green}.log`). Canned Go events use an external-tool stub;
  engine qualification comes from the real journeys, not that stub.
- Replacement native/shared assertions passed before old tests were removed.
  Fixture-format and native-test-oracle errors during development are retained
  in `batz-replacement-native.log` and earlier model logs; corrected results are
  `batz-preview-strict-green.log`, `batz-native-models-green.log` and
  `batz-cloudtrail-source-green.log`.
- One composed `ci/test all-local` passed: `batz-composed-final.log` (45.658 s),
  including fast tests, tagged vet, shared Go/native Python, native model tests,
  actual adapters and the single production E2E. All four moved functions
  (preview, explicit example, strict partial, growing-directory replay), plus
  recovery, appear as executed JSON passes, with no skips. Native model selected
  and executed IDs match exactly. A subsequent additional coarse-time/fraction
  row was checked with only `ci/test models aws-cloudtrail --mode native`
  (`batz-coarse-time-green.log`, exit 0); no runtime code changed or full suite
  was repeated for it.
- Real composed-catalog selector checks in a private temporary Git history
  (`batz-real-selection.log`): CloudTrail model-only/E2E-only selects CloudTrail
  models/adapter/E2E, `app=false`; core retry selects `app` without E2E;
  shared helper selects `app` plus the first production E2E; internal fixture
  model selects its native/adapter checks and `app`, with no synthetic E2E.
  The existing selector suite also checks an independent new pipeline, native
  tests only, removed edges and docs-only changes.
- One-time source inspection finds CloudTrail references in shared-test source
  only in `aws_cloudtrail_e2e_test.go`; default fixture-copy paths contain no
  production catalog. Diff whitespace and local documentation targets checked.

| Local cost, warm installed pins, no Go result caching | Before | After |
| --- | --- | --- |
| CloudTrail E2E package/body | 31.612 s package, five tests | 2.958 s package / 2.68 s body, one test (3.64 s body in focused run) |
| Shared application Go command | 17.422 s | 42.856 s |
| Shared run/scratch Go command | 10.588 s | 12.974 s |
| Shared Python contracts command | 8.907 s | 11.526 s |
| Synthetic standalone E2E package | 4.299 s | removed; original assertions reused |
| Batched model/adapter command | 2.560 s | 1.793 s |
| Sum of test/check command elapsed times (includes compilation/import; not CPU time) | 86.639 s | 85.281 s |
| Collector build per entry-point preparation | 2.432–2.889 s | 2.688 s in composition |
| Composed all-local wall | not freshly measured before (separate lane baselines) | 45.658 s |

The before lane walls (6.145 + 22.052 + 36.348 + 5.083 s) are separate runs,
not a comparable all-local critical path. Likewise, three baseline wrapper
builds versus one composed build are not a new implementation saving: all-local
already reused one build before. Dependency preparation was not repeated locally;
tools were installed. Command minus package time combines Go compilation/startup
and cannot isolate CPU build cost. The sum-of-command comparison is only about
1.6% lower, within host/concurrency noise; no material total-work improvement is
claimed. Fast Go command changed from 6.055 to 6.480 s under composition; this is
not a separately timed after `fast` lane.

Affected app/preview invocations fall from 13 (CloudTrail 12 + synthetic 1) to 12
(shared 11 + CloudTrail 1). Completed model executions across those affected
journeys fall from 11 to 10; per-file collector executions from 14 to 13. The
remaining existing shared recovery/process, direct storage and adapter executions
are retained. Canceled model/discovery attempts and native graph discovery are
separate from completed model execution; this is a code-path count of the changed
journeys, not an OS-level inventory of every process in all-local.

The blueprint meets the additional <=10 s warm-body target. All-local is within
S3.1's owner-approved 25% tolerance around 45 s. **The shared-app <=20 s target
(with 25% tolerance) is not met:** moved native proof now dominates that lane.
This remains an acceptance limitation, not a reason to drop safety assertions.
A next bounded simplification would share successful native preview inspection
with repeatability/explicit association and combine compatible file-history and
recovery transitions before adding concurrency or new job machinery. That needs
a distinct coverage-preserving design; it was not added to this cleanup.

Hosted before evidence remains CloudTrail 243 s and synthetic 136 s jobs (with
preparation/build breakdowns above); the previous review recorded about 158 s
for shared-runtime. No after commit was pushed and no hosted run was triggered,
so cache save/restore, whole-job totals and PR queue/critical-path improvement
remain **unmeasured**. Removing a VM's repeated setup is a structural reduction,
not a measured PR-latency claim. Exact cache-hit keys remain frozen within a PR;
first runs of other PRs remain cold. Hosted qualification and acceptance of the
shared-lane budget miss remain owner decisions.

Final-review cost clarifications: the shared Go/Python after times above are
components of contended all-local, whereas their before times are standalone
lanes. They are not like-for-like standalone lane timings. The moved tests alone
take 26.23 s in that run, exceeding the 25 s shared target tolerance. Pipeline-only
PRs shed the shared journeys; core-only PRs gain both that coverage and its cost
(they previously did not select the old CloudTrail-tagged tests). Standalone after
fast and complete one-pipeline workload timings remain unmeasured; composed
components suggest the pipeline target is met, but that is inference. Counts
remain explicitly scoped to changed journeys, not all selected OS processes.

## Final Claude review and parent correction

[Complete final findings and dispositions](../reviews/2026-11-04-pipeline-e2e-final.md)
are retained: one Medium final-row conflict/lineage oracle gap and nine Low items
were fixed in one bounded correction. Native final-output assertions now check
complete structures, without helper models or application executions. Bare E2E
selection, independently selected replay, prose-only fixture notes and readable
shared logs are corrected. The unused second expectations file was removed.

After that correction, the affected CloudTrail native suite passed (ten selected
and executed tests, 1.530 s command); the correction composition passed fast
checks/tagged vet, all 22 selector tests and the three shared journeys with new
assertions (`/tmp/batz-correction-check.log`, exit 0). No second all-local was run.
The 45.658 s all-local timing above belongs to the reviewed composition before
these bounded corrections, not a claimed fresh benchmark of every final byte.
Unchanged runtime checks retain their evidence; affected corrections have fresh
focused results. Full after standalone workloads, whole-suite start inventory,
shared-budget acceptance and hosted cache/PR wait qualification remain open as
stated above. Final file/dependency hashes are `/tmp/batz-delivered-inputs.json`.
