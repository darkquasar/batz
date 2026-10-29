# Native reviewer /root/s1_boundary_review — complete response

Assessment: **blocked at the S1-A/B boundary**. The review found two Major findings and five smaller contract gaps. This is not whole-plan review R or publication authority.

```text
+----------------+  verify files  +------------------+  native evaluation  +---------------+
| Child receipt  | ============> | Committed inputs | ==================> | Built outputs |
+----------------+               +------------------+                     +---------------+
                                                                                 |
                                                                     assertions  |
                                                                                 v
                                                                         +---------------+
                                                                         | Publish gate  |
                                                                         +---------------+
```

The receipt gate is substantially stronger than the publication gate: publication checks fresh metadata and counts but does not establish exact output membership and complete lineage.

1. **S1BR-001 — Major — Publication accepts substituted observations and invalid origin references.**

   Evidence: [runner.py:181](/Users/martinpopolito/GitHub/batz/runtime/sqlmesh/runner.py:181) compares output count with the staging-derived mapped count. Its lineage query at line 184 joins only `observation_id`; it never compares the expected and actual observation multisets or verifies `parsed_revision`, `file`, and `row`. Fresh digest/nonce checks at lines 173–178 do not establish those facts. [run.go:601](/Users/martinpopolito/GitHub/batz/internal/run/run.go:601) subsequently accepts the result after identity/file checks.

   Concrete counterexample by source inspection: with two supported observations, modify the native `api_activity` model to emit the first observation twice and omit the second, preserving its payload and current build context. Count, source join, actor/action/time, digest and nonce checks all still pass. Independently, replacing an origin’s file/revision/row while retaining its observation ID also passes.

   Consequence: publication can claim exact committed membership while silently omitting evidence or exposing unresolvable lineage.

   Correction: compare exact expected and exported observation multisets, enforce the mapping’s one-to-one cardinality, and resolve the complete origin tuple against committed file membership and actual row positions. Add native negative cases for duplicate-plus-omitted observations and independently corrupted origin components, preserving the previous reader on failure.

2. **S1BR-002 — Major — Accepted resource maxima remain unenforced.**

   Evidence: [run.go:44](/Users/martinpopolito/GitHub/batz/internal/run/run.go:44) reports collector/model tree memory and scratch limits. The child launcher at line 383 applies timeouts/process-group cancellation but no memory or aggregate disk enforcement. [runner.py:68](/Users/martinpopolito/GitHub/batz/runtime/sqlmesh/runner.py:68) configures DuckDB memory only. The accepted contract explicitly requires these bounds at [first-tracer-contract.md:163](/Users/martinpopolito/GitHub/batz/docs/contracts/first-tracer-contract.md:163).

   Consequence: a run can exceed the accepted process-tree or private scratch/output maxima and still publish.

   Correction: retain this blocker until the original maxima are qualified or the owner explicitly accepts an amended invariant followed by its required native qualification. The sampled-watchdog proposal is not accepted or waived by this review. The retained rlimit probe demonstrates rejection of those attempted mechanisms, not impossibility of all enforcement.

3. **S1BR-003 — Medium — SourcePipeline decoding does not enforce the accepted schema.**

   Evidence: [app.go:104](/Users/martinpopolito/GitHub/batz/internal/app/app.go:104) uses `KnownFields` and selected semantic checks, but never validates required-field presence or identifier constraints. A missing/empty `name` or omitted `params.path.required` passes this code. [explain.go:67](/Users/martinpopolito/GitHub/batz/internal/app/explain.go:67) has the same gap and can emit an invalid ExplainReport. These fields are required by the frozen SourcePipeline schema and [first-tracer-contract.md:33](/Users/martinpopolito/GitHub/batz/docs/contracts/first-tracer-contract.md:33).

   Consequence: invalid documents can reach workspace mutation/execution, and static output can violate its own schema.

   Correction: validate the complete decoded document against the accepted schema, preserving duplicate-key rejection and field/source diagnostics, before resolving assets or mutating the workspace. Test omitted required fields and invalid identifiers through both commands.

4. **S1BR-004 — Medium — Several mapped source fields silently coerce invalid types.**

   Evidence: [source_staging.sql:27](/Users/martinpopolito/GitHub/batz/normalization/models/source_staging.sql:27) checks types for ARN, recipient account, bucket and key only. [api_activity.sql:8](/Users/martinpopolito/GitHub/batz/normalization/models/api_activity.sql:8) extracts `eventID`, `awsRegion`, and `userIdentity.accountId` as strings without equivalent validation. A supported record containing numeric/object values in these fields is classified `mapped`; extraction converts their representation into canonical string fields.

   Consequence: malformed supported records can be published with complete quality and invented string interpretations instead of mapping rejects, contrary to [first-tracer-proposal.md:91](/Users/martinpopolito/GitHub/batz/docs/contracts/first-tracer-proposal.md:91).

   Correction: validate every consumed mapped source field’s accepted type and distinguish absence/null from invalid supplied values. Add native negative fixtures for the currently unchecked fields.

5. **S1BR-005 — Medium — Failure conversion loses accepted diagnostic categories and exits.**

   Evidence: [run.go:440](/Users/martinpopolito/GitHub/batz/internal/run/run.go:440) always constructs diagnostics with category `execution` and code `report.Outcome`, even for a typed integrity or busy failure. Busy acquisition at line 463 therefore reports code `running`. Model assertion failures from [runner.py:179](/Users/martinpopolito/GitHub/batz/runtime/sqlmesh/runner.py:179) return through [run.go:590](/Users/martinpopolito/GitHub/batz/internal/run/run.go:590) as generic exit 1 rather than required-check exit 4. Signal cancellation has no conversion to exit 130. Preflight diagnostics at [app.go:47](/Users/martinpopolito/GitHub/batz/internal/app/app.go:47) are unstructured strings.

   Consequence: callers cannot reliably distinguish cancellation, failed assertions, integrity failures and ordinary operational failures using the accepted interface.

   Correction: carry stable category/code/location through failures; distinguish child assertion outcomes from execution errors; convert cancellation explicitly; emit the agreed diagnostic structure for preflight failures. Assert exact exits/categories in negative tests.

6. **S1BR-006 — Medium — Unrepresentable timestamp precision/range becomes successful partial publication.**

   Evidence: [source_staging.sql:22](/Users/martinpopolito/GitHub/batz/normalization/models/source_staging.sql:22) treats both more-than-nine fractional digits and failed `TIMESTAMP_NS` conversion as ordinary mapping rejects. [run.go:636](/Users/martinpopolito/GitHub/batz/internal/run/run.go:636) publishes those rejects as partial, with default exit 0. The accepted proposal specifically requires an owner tradeoff for source times outside the encoding range or precision at [first-tracer-proposal.md:95](/Users/martinpopolito/GitHub/batz/docs/contracts/first-tracer-proposal.md:95).

   Consequence: the implementation silently chooses rejection as its representational policy instead of stopping acceptance for the specified decision.

   Correction: distinguish malformed timestamps from valid but unrepresentable timestamps and fail the latter before normalized publication pending the required owner disposition. Retain parsed evidence and the previous normalized revision.

7. **S1BR-007 — Low — Implemented child handoff differs from the accepted contract.**

   Evidence: [run.go:386](/Users/martinpopolito/GitHub/batz/internal/run/run.go:386) supplies no `BATZ_*` variables. The collector receives a context-file argument at line 526 and decodes it at [collector main.go:30](/Users/martinpopolito/GitHub/batz/cmd/batz-collector/main.go:30). The accepted contract promises generated identity/stage/source variables plus a read-only context document at [first-tracer-contract.md:141](/Users/martinpopolito/GitHub/batz/docs/contracts/first-tracer-contract.md:141). The context is currently created writable by its owner.

   Consequence: the current adapter works, but the declared `env: BATZ_SOURCE_PATH` delivery contract is not actually implemented.

   Correction: implement the accepted handoff and test it, or obtain an explicit contract amendment describing the context-only protocol. This is not evidence of a current reserved-value override.

Strengths supported by the inspected source:

- [run.go:176](/Users/martinpopolito/GitHub/batz/internal/run/run.go:176) verifies receipt identity, closed file hashes, exact Parquet schema/counts, record provenance and source accounting; [wire.go:12](/Users/martinpopolito/GitHub/batz/internal/run/wire.go:12) rejects duplicate, unknown and missing wire fields.
- Parsed placement precedes SQLite commit; normalized publication uses a separate revision and conditional pointer update. [publication_test.go:36](/Users/martinpopolito/GitHub/batz/internal/app/publication_test.go:36) and retained native results support preservation of parsed evidence and the previous reader after model failure.
- Project file scanning occurs before collection/framework discovery, and copying repeats the extension/symlink checks: [app.go:156](/Users/martinpopolito/GitHub/batz/internal/app/app.go:156), [run.go:408](/Users/martinpopolito/GitHub/batz/internal/run/run.go:408).
- Native restatement and nonce-bearing evaluation evidence exist, including checked empty input: [runner.py:109](/Users/martinpopolito/GitHub/batz/runtime/sqlmesh/runner.py:109).
- The narrow owner boundary uses nonblocking `flock`, passes the lease into children, and preserves it across SQLMesh subprocess launch. The existing owner test establishes busy refusal without acquisition. Child death/reparenting recovery remains unqualified here.

Reviewed identity and evidence:

- Root: `/Users/martinpopolito/GitHub/batz`; branch `main`; base and HEAD absent. Git status confirms untracked delivered source.
- Boundary package SHA-256: `8995920b0e41f85d166e4f21a1be3d46a28aff9e306feb9d2769ff2b1a769f7f`.
- Source snapshot SHA-256: `15069d0b00fe4dc7994412d85c19149e031be5933fd78c8c701d05907c3381df`.
- Plan SHA-256: `6ccb5b5afa0e4f8d6f571df178d06a706683e8922a767f18702e8fc305ef69c3`.
- Spec SHA-256: `6cb30a343990eb6cc9a2f785d09f6b71a4831123453b844b77a473c985029ba4`.
- I independently hashed all **43 source files and 55 supporting entries** against the exact per-file hashes in [checked-source.json](/Users/martinpopolito/GitHub/batz/docs/contributing/evidence/s1/checked-source.json) and [boundary-review-inputs.json](/Users/martinpopolito/GitHub/batz/docs/contributing/evidence/s1/boundary-review-inputs.json). All matched, including an end-of-review recheck. Those manifests identify the exact reviewed bytes; large dependency checksum inventories were identity-checked rather than independently auditing every dependency.

Actual checks were read-only `cat`, `nl`, `sed`, `rg`, standard-library SHA-256 verification using `/usr/bin/python3 -I -B`, and Git metadata with `GIT_OPTIONAL_LOCKS=0`. Reads and hash checks exited 0. `git rev-parse --verify HEAD` exited 128 as expected for the unborn branch.

I inspected the retained aggregate test/build/vet/module-integrity results, CLI query/report, schema-validation record and negative-control evidence. Their recorded commands pass; **I did not rerun them**. No tests, installations, network calls, workspace mutations or native fault injections were performed. New counterexamples above are source-derived findings, not claimed executed reproductions.

S2–S9 isolation, recovery, reconciliation, remote acquisition and portability remain outside this boundary review. Resource-policy acceptance remains pending. No descendants or background jobs were created.

## Coordinator disposition — one correction batch

The native reviewer completed and created no descendants/background jobs. Its
complete original response above is retained unchanged. The coordinator verified
the cited source and reproduced the defects before correction. This is the one
bounded correction/disposition batch under plan-execute-cx and
receiving-code-review-cx. Corrected bytes are **parent-verified, not independently
re-reviewed**; no successor reviewer was launched.

| Original ID / severity | Disposition | Retained correction evidence |
| --- | --- | --- |
| S1BR-001 / Major | Fixed: exact staging and mapped observation multisets, complete origin tuple, payload and one-to-one cardinality are checked before publication. | review-001-red.log / review-001-green.log; final native duplicate/omit and file/revision/row negative tests preserve the prior reader. |
| S1BR-002 / Major | Resolved by explicit owner amendment plus native qualification of that amended policy. The owner allowed brief overshoot, then instructed RAM monitoring only with no cutoff. No original hard-max qualification is claimed. Current scratch threshold stops/settles real descendants; timeouts remain enforced. | s1-macos-budget-decision.md; ram-monitor-red.log / ram-monitor-green.log; resource-supervisor-red.log / resource-supervisor-green.log; final native/race checks. |
| S1BR-003 / Medium | Fixed: both commands use the embedded complete SourcePipeline schema. The already-pinned validator handles its shared Draft-7/2020-12 vocabulary; an explicit guard refuses unsupported schema keywords. No external schema load or general 2020-12 support claim. | review-003-red.log / review-003-green.log; final omitted-field, invalid-ID, duplicate/unknown-field and bare-explain tests. |
| S1BR-004 / Medium | Fixed: the remaining consumed mapped string fields reject invalid supplied types; absence/null stays distinct from malformed input. | review-004-red.log / review-004-green.log; final native mixed-field-type test retains five mapping rejects and one mapped record. |
| S1BR-005 / Medium | Fixed: structured preflight diagnostics, receipt integrity, busy/blocked, required assertion exit 4, and cancellation exit 130 retain stable categories/codes. | review-005-red.log / review-005-green.log; binding-cancellation-checks.log; final actual SIGTERM test after native model activity preserves parsed input and releases the owner lease. |
| S1BR-006 / Medium | Fixed: valid out-of-range/overprecision times return explicit timestamp-representation-unavailable exit 3 before normalized publication, retaining parsed data and the prior reader. Malformed times remain record rejects. | review-006-red.log / review-006-green.log; representation-diagnostic-red.log / representation-diagnostic-green.log; final precision/range, nano/offset and strict-partial checks. |
| S1BR-007 / Low | Fixed: coordinator-owned BATZ environment values are delivered and the context document is mode 0400. Reserved overrides remain rejected. | review-007-red-valid.log / review-007-green.log; final actual native YAML environment-interpolation and context-mode test. Initial review-007-red.log/red-diagnostic.log used an unavailable Bloblang function and are retained as test-preparation failures, not intended RED evidence. |

No source finding was downgraded or omitted. No unresolved finding remains within
the amended narrow S1 boundary. This does not accept S2–S9, coordinator-death
recovery, arbitrary hostile native code, Linux/container support or final R.

Reviewed source snapshot SHA-256:
15069d0b00fe4dc7994412d85c19149e031be5933fd78c8c701d05907c3381df.
Corrected source snapshot SHA-256:
088cbb09d9a0d345cb913c8a9606c77febcfba30b4de6cf01f911ecb1b33d10a.
`correction-delta.json` retains every old/new per-file hash (26 changed/new entries).
`corrected-source.json` covers 54 exact source/check-script files. Base/HEAD remain
absent on unborn main; no commit or Git mutation was made.

Fresh final check results SHA-256:
7274902916ee2a47a8055dcbfdcaebeff179574929b99b0dc83c1500bb92f453.
`corrected-check-results.json` records full commands/options/environment/exits and
readable logs. Build, complete Go tests, supervisor/receipt race tests, vet, module
integrity, 34 contract checks, real CLI run, ordinary SQL comparison and static
JSON explain all exit 0. `corrected-native-validation.json` additionally validates
actual run/receipt/inventory/explain documents and independently checks canonical
membership agreement across the Go producer and Python consumer. Source hashes
were rechecked unchanged after these commands.

Latest small-fixture model measurements: 34 successful samples, 234,965,835-byte
sample mean and 488,865,792-byte sampled peak, no missing samples; elapsed 3403 ms.
The collector finished in 30 ms and supplied only one early sample, so its value
is not a reliable sizing average. Summed RSS can double-count shared pages and
sampling can miss peaks. These observations are not production defaults or a
scale claim. RAM has no Batz cutoff; native scratch/resource stops remain failures
before publication. All observed data and case artifacts remain retained.
