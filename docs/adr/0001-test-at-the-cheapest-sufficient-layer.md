# ADR-001: Keep the test surface small and compositional

Status: Accepted 2026-10-29; amended 2026-10-30, 2026-11-04 and 2026-11-05 by owner direction.
Scope: Batz application, pipeline development and CI.
This dated amendment extends the original cheapest-sufficient-layer decision;
it replaces the blanket testing strategy with explicit ownership/selection rules.
The prior accepted text remains in Git history; the shared precision promise stays.

## Context

Repeated Bento/SQLMesh/DuckDB startup dominates test time even with one-record
fixtures. Copying application lifecycle tests for every pipeline, or running the
whole stack for each field/report nuance, makes growth unaffordable. A passing
full pipeline is useful wiring evidence; it is an expensive default test layer.
This amendment retains the original cheapest-sufficient-layer decision and makes
ownership, selection and parallel execution explicit.

## Decision

**Test at the cheapest sufficient layer. Prove each shared guarantee once at its
cheapest sufficient interface. New pipelines add distinct source/mapping evidence
and representative wiring coverage when justified; they do not duplicate the
application test suite.**

| Behavior | Sufficient proof |
| --- | --- |
| Parsing/selection, diagnostics, types, counts, quality and strictness decisions | Direct calls with tiny invented inputs |
| Durable commits, inventory, CAS, idempotency, recovery and status | Real filesystem/Parquet and SQLite through production helpers |
| Ownership, interruption, cancellation and isolation | Small real processes/OS checks at deterministic critical transitions |
| Source framing/decoding and retained payload fidelity | Actual adapter/collector through parsed evidence |
| Pipeline mapping semantics, rejects and unsupported coverage | Batched native SQLMesh model tests with reviewed fixtures |
| Engine-native types and timestamp precision | Explicit native SQL/reader probes grouped with existing execution |
| Application wiring and model-only recovery | One representative real shared journey |
| Pipeline native contract | Actual adapter fixture through declared roots, required checks, production profile and exact origin/accounting validation |

```text
+------------------+  reuse proof  +---------------------+
| Shared contracts | ============> | Pipeline checks     |
+------------------+              +---------------------+
                                            |
                                            | add distinct evidence
                                            v
                                  +---------------------+
                                  | Native contracts    |
                                  +---------------------+
```

1. Test through interfaces used by production. Use real storage/processes where
   durability or ownership is the claim. Invented evidence does not prove native
   values, transport support, engine compatibility or sandbox enforcement.
2. Group compatible assertions in one result/journey. Keep separate fresh runs
   only when freshness, repeatability or isolation is what they prove. Most
   fixtures need one or two distinguishing records; large acceptance examples
   run once at the sufficient layer, not once per pipeline or flag combination.
3. A pipeline native contract proves its named route and fixture, not all source variants.
   Transport-only routes reuse mapping proof and test actual acquisition/fidelity
   separately. A different pipeline model graph requires native contract proof.
   Targeted core interaction proof uses shared synthetic inputs; routine source
   contributions require no application E2E. Any specifically justified pipeline
   qualification belongs with its owning pipeline repository.
   Preserve specifically promised real transport/platform qualification, including
   local/R2 normalized parity. Avoid blanket feature/pipeline/route/platform products.
4. Use native SQLMesh tests for mapping breadth, not a new test framework or fake
   engine. Default DataFrame equality may hide dtype/time conversion loss: preserve
   explicit native `typeof`/`epoch_ns` probes. One shared precision probe is enough
   unless source interpretation differs; preserving precision is not clock accuracy.
5. Local development and CI call the same commands, fixtures and assertions. CI
   selects/distributes jobs; it does not own another suite. Pipeline-A-only changes
   run fast app contracts and A's tests, without starting B. Shared changes broaden
   affected contracts and representative compatibility checks, not automatically
   every pipeline E2E. Release/scheduled qualification composes the wider catalog.
6. Run independent selected jobs concurrently with isolated workspaces/state and
   a resource cap per runner. Avoid nested oversubscription, global fixture edits,
   order dependencies and mutable shared SQLMesh state. Batch model cases to
   amortize startup. Requested lanes fail on missing tools; tagged code is still
   compiled/vetted. No silent skip is qualification.
7. Replace expensive duplicates after replacement evidence is green. Explicitly
   record any retired promise in the existing plan/handoff. Keep data preservation,
   exact accounting, publication isolation, child ownership/cancellation and privacy
   checks. Do not add a registry, fixture-cache service, generic crash framework,
   per-test review ceremony or a separate coverage ledger.
8. Measure real cost when it grows: fixed before/after workload, native starts,
   total work and wall time. Do not hide unchanged local work in a slow lane or
   claim success from cached test results. Report setup/cold builds separately. The
   owner removed hard test-time, reduction and tolerance targets on 2026-11-05: keep costs low, investigate meaningful
   changes and simplify redundant work without a numerical pass/fail gate. Preserve
   required correctness proof and finite execution/resource safety limits.

Before adding a test, identify the new failure it catches, the cheapest sufficient
layer, and whether an existing fixture/journey can carry the assertion. These are
review questions, not a new approval process or mandatory per-test paperwork.

### Owner amendment 2026-11-04: each pipeline's contribution

Shared framework tests own lifecycle, crash/recovery, ownership and publication
guarantees. A routine pipeline addition reuses those guarantees without repeating
their test matrices. It contributes focused source parsing, adapter/acquisition,
mapping, time interpretation and identity evidence, plus representative wiring
where an integration risk justifies it. Source-specific behavior needs its own
distinguishing evidence. Routine additions do not require catalog-wide E2E;
selection follows rule 5. Reuse existing fixtures/journeys for compatible assertions.

"Cheapest" still means sufficient to prove the claimed boundary: the real
process/storage, native transport and precision requirements above remain in force.

## Consequences and evidence

Application contracts stay bounded as pipeline count grows; mapping evidence grows
with distinct semantics, and costly journeys grow with justified integration risks.
Pipeline SQL owns vendor semantics; Batz owns common evidence/publication integrity.
SQLMesh owns model lifecycle and its native testing facilities.

Earlier selected checks fell from seven native runs/28.16 s to three/11.57 s;
eight expectation checks used one Python process/0.04 s. Those observations
support startup reduction, not a universal latency or production-scale promise.
Local cost measurements and hosted CI execution are separate qualifications.
The [CI guide](../usage/ci.md) describes selected jobs and setup costs.

### Owner amendment 2026-11-05: shared application proof

S3.2-CI supersedes the routine per-pipeline application E2E requirement. One
representative shared synthetic journey owns run-to-reader behavior; focused
real-boundary safety tests remain. Actual pipeline adapter/native contracts own
source-to-model proof without app workspaces, publication or readers. Native YAML
test-only edits select native tests, even for the synthetic pipeline. Shared SQL
selects native consumers and app only if consumed by the shared journey. Core,
collector, runtime, runtime fixture and relevant global/dependency changes select
shared app coverage. Optional identity and explicit transport/platform promises
retain their prior meaning.
