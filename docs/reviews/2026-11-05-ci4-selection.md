CI4 selection/coverage audit is ready for lead disposition: **3 Medium findings**, no Critical or Major findings in the checked scope. I made no source/report/index changes, ran no experiments or tests, and started no background work or descendants.

Exact subject:

- Root: `/Users/martinpopolito/GitHub/batz`
- Base/HEAD: `b361019c0dd527cb156dd8697cbf153074e0b670`
- `/tmp/batz-ci-follow-on/ci4-inputs.json` SHA256: `8ddf128fa0316d08eaa860e363317a7ae0d2e0371983a98f26171f9fd69bdf87`
- `/tmp/batz-ci-follow-on/ci4.patch` SHA256: `3976eb55420166358f5b0c2161eebb28ab5aca6952a0335dc9822db9c5568bc6`
- Verified all 138 manifest entries against current files before and after inspection: **no drift**.

```text
+--------------------+  select  +----------------------+
| Base/head changes  | =======> | Native / adapter / app|
+--------------------+          +----------------------+
                                          | results
                                          v
                                +----------------------+
                                | Workflow gate        |
                                +----------------------+
```

**SEL-1 — Medium: production manifest validity is no longer checked for pipeline-only contributions.**

Locations: `runtime/sqlmesh/test_models.py:33`, `:84`, `:154`, `:170`; `ci/pipelines.py:16`; contrast `internal/app/pipeline.go:62`–`:134` and `schemas/v0alpha1/source-pipeline.schema.json:202`–`:214`.

The native runner loads raw YAML. `manifest_contract()` constructs a valid execution-contract version itself, while `pipeline_contract()` treats an example’s expectations as optional. `working_catalog()` checks only discovery identity/version, not the complete SourcePipeline schema. Shared app tests now always copy synthetic manifests (`internal/app/publication_test.go:13`–`:24`), so fast/shared checks cannot validate production manifests incidentally.

Consequently, removing a required expectations reference or introducing an invalid input parameter declaration can leave native/adapter/fast checks passing while production `loadPipeline()` refuses the manifest. The captured broken `wiring` example demonstrates why this boundary matters; adding its missing file repairs that instance without closing the general verification gap.

Correction: validate selected manifests with the existing production SourcePipeline schema before native/adapter execution, including production’s supported `version == 1` constraint and native project-reference requirement. The existing pinned `jsonschema` dependency can do this without a Go application execution or mirrored schema. Add cheap positive/negative tests for required expectations and another invalid manifest field; native execution is unnecessary for schema refusal.

Rejected alternatives: restoring a per-pipeline app E2E, duplicating the schema in Python conditionals, or validating only CloudTrail.

**SEL-2 — Medium: overlapping adapter contracts apply a potentially stale plan before refreshing shared ancestors.**

Locations: `runtime/sqlmesh/test_models.py:123`–`:138`, `:197`–`:199`; pinned SQLMesh `core/context.py:1777`–`:1793`.

For a second selected owner, the runner replaces `batz_input.parsed`, then applies a normal plan. Only afterward, when selected models overlap previously evaluated models, it applies a restatement. Consider A and C sharing an input-dependent materialized ancestor: A has already populated its intervals; C introduces new roots with a required audit. C’s first plan can reuse A’s ancestor while evaluating/auditing C’s new roots. An audit can fail before the corrective restatement executes, although C would pass from fresh state.

This is a source-derived failure mechanism, **not an experimentally reproduced result**. Existing `NativeBatch` coverage tests native unit-test selection, while `PipelineContract` tests one owner’s failed required checks; neither exercises actual two-owner adapter contracts sharing materialized input-dependent SQL.

Correction: ensure the first applying plan for each fixture includes both fresh shared input evaluation and any newly selected owner models. The pinned API exposes `always_include_local_changes`; its source explicitly controls the default behavior that otherwise ignores local changes during restatement. Verify that API and use one combined plan where supported. Add one invented two-owner/shared-ancestor regression with an attached audit that rejects another fixture’s data, proving both invocation orders or an equivalent distinguishing refresh transition.

Rejected alternatives: retaining two applying plans and hoping the second repairs the first, disabling audits, or dropping current freshness/membership validation. Independent adapter contexts are a safe fallback design if justified, but their additional Context cost should be recorded.

**SEL-3 — Medium: native-YAML-test-only edits still select the full fast lane.**

Locations: `ci/select.py:209`–`:218`; `ci/native_select_test.py:75`–`:80`, `:243`–`:249`; `docs/usage/ci.md:88`.

For an attributable native-test-only edit, `selected` contains its owner, so `fast = bool(selected) or ...` is invariably true. The new cases verify `app == false` and empty adapters, but never verify `fast == false`.

ARC-012 explicitly requires native SQLMesh YAML test-only edits—including synthetic tests—to select the native test batch only. Current documentation repeats that rule. The additional fast lane runs Go tests, tagged vet and entrypoint checks, with another hosted job/tool preparation; the retained standalone CI2 fast observation is 9.885 seconds. That duration includes its observed compilation/cache conditions and is not a universal warm penalty.

Correction: suppress fast only when all relevant executable changes are fully attributed native YAML tests. Preserve fast for mixed, source SQL, core, selector/gate, global and unattributed changes. Extend the existing production and synthetic test-only cases with `fast == false`, plus a mixed-change control.

Rejected alternatives: removing fast from pipeline SQL/config/fixture changes, hiding fast work in another lane, or changing documentation to weaken the approved selection contract.

The gate correctly requires selection success and rejects reported failed/cancelled dependency jobs (`.github/workflows/pr.yml:140`–`:155`). It does not independently compare selected flags with successful job results, so unexpected selected-job skips remain a hardening opportunity. I found no current normal-workflow trigger establishing that as an additional concrete defect; it is **not counted** among the three findings.

Supported strengths:

- Production E2E ownership, filename/tag enforcement, workflow matrix and all-local loop are removed.
- Native base/head impact preserves former consumers, and synthetic shared-app impact is calculated before expanding test-only ownership (`ci/select.py:168`–`:181`).
- Production-private SQL and independent pipeline additions exclude shared app coverage in the existing selector cases.
- Shared native dependencies without a synthetic consumer exclude app; removed dependencies reaching a synthetic runtime closure include it.
- `run_selected_tests()` compares selected IDs, successes, run count, failures and skips (`test_models.py:89`–`:100`).
- Shared Go execution rejects missing execution, selected skips, missing package success, an empty requested package and disappearance of the representative journey (`ci/test.py:182`–`:226`).
- Actual adapter output feeds all declared native roots. The checker calls production `validate_contract`, `check_build` and `verify_published`, including native profile and exact origin/payload accounting.
- CloudTrail’s source-owned tests retain complete final conflict/origin structs and now distinguish the two native nanosecond/offset projections. Shared run-to-reader proof uses invented content and checks exact native epochs, payload and parsed-revision origins in the recovery journey.

Observed evidence, read rather than rerun:

| Evidence | Retained result |
| --- | --- |
| `ci2-results.json` / `ci2-pipeline.log` | Pipeline command exit 0, 5.019 s |
| `ci2-results.json` / `ci2-native.log` | Independent native mode exit 0, 1.261 s |
| `ci2-shared.log` | Shared command exit 0, 41.781 s; required shared journey passed; no selected Go skips |
| `ci2-fast.log` | Fast exit 0, 9.885 s |
| `runner-green.log` | Eight entrypoint/Go-event tests passed |
| `selection-green.log` | Twenty-five selector tests passed |
| `ci2-final-source.log` | Final ten CloudTrail native tests executed successfully, with selected/executed identities equal |

I used read commands (`cat`, `nl -ba`, `sed`, `rg`), `git rev-parse HEAD`, and read-only Python hashing of the supplied manifest. I inspected the approved ARC-010–013/CI1–CI-R requirements, repository instructions/ADR, selectors/runners/workflow, source/native/shared proofs, schemas and relevant pinned SQLMesh implementation. No hosted run, cache qualification, process inventory or new timing measurement was performed. Existing timing misses, absent hosted evidence and archive authentication limits remain open.

Principal reviewed file hashes:

```text
ci/select.py
23477c36d40212df6999d4f7cba9ed9ec04bbfebc56af0d48a9b7951049e4f12
ci/test.py
d45519ca56ee83d896eb3c4caf04c4c2f6696a2fe84163d91fe6e8d9b4aff642
ci/pipelines.py
5879de4d2238a600f3c32e869ceb5af47958eb70a8ffae2dc52a1b9a9d56449c
ci/native_select_test.py
3fe787b8d9ca49763afccdd301bf898caf9754cded2f1c7a4241a60fb521dc1d
ci/test_entrypoints.py
38d77b2e3436b0e46f4bd342c3e2b448fee429e4ea997b54bcbcd6f6f83d76df
runtime/sqlmesh/test_models.py
fb53e8d96d969818e610442fbd577466483f24e1a7f9a24f5e5fc878f42efda7
runtime/sqlmesh/test_scope.py
d64005dd84f1f89fe0360483c135d616193cfa84122c7cbc41ac1162ad18447b
.github/workflows/pr.yml
e18f4d3df5d02e7529401eabc3ee625435708bd79be86e3b38c12bbf7989ff10
internal/app/app_runtime_test.go
dc0ce5a94597802872043b455030f4533fd6cd1debc879be1b9ffb5343a3cbaf
internal/app/publication_test.go
ddf902377b39071cb4f2f0ca1806eec91ad374752f9d9ece59d889914aad7147
tests/aws-cloudtrail/test_mapping.yaml
1a44a95c6a3ca2c2344a9be72a5f6ebd807ff55c4d4121d74df2babec43d8447
pipelines/aws-cloudtrail/pipeline.yaml
a5fad8e68487d620896814cec62d812bbbd445b17596bd49bb29f6d4b1f7ccb5
pipelines/aws-cloudtrail/fixtures/wiring.expected.json
3af22f79f788611ac960762d237c9723701e4c7487ebeec7bbd3eda4097fe0d9
```

Return status: audit complete and settled; three Medium findings require bounded correction or owner disposition. No implementation acceptance, commit, push, cleanup or successor review is implied.
