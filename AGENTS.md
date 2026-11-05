# Repository working instructions

## Pipeline contribution contracts

Use the [contributor guide](contributing.md) and [native contract criteria](docs/usage/pipeline-e2e.md).
Routine pipelines supply source/config/fixtures, native SQL/YAML tests and actual
adapter/native contract proof. No per-pipeline application E2E, Go test/tag or
shared lifecycle matrix is required. Shared app tests use only invented internal
content. Private pipeline SQL selects native consumers/contracts plus fast checks;
shared SQL selects app only when the synthetic journey consumes it. Native test-only
edits select native tests, including synthetic tests. Core/runtime/collector/global
inputs select shared coverage. Optional event identity must not block valid input.

## Proportionate verification

Follow [ADR-001: Keep the test surface small and compositional](docs/adr/0001-test-at-the-cheapest-sufficient-layer.md).
**Test at the cheapest sufficient layer.** This applies to both adding tests and
choosing which checks to run during a task.

- Select checks for the changed behavior and affected contracts. Briefly state
  the selection and reason in the normal progress update; no separate ledger or
  approval step is required.
- For prose-only changes, inspect the diff and relevant links/formatting. Do not
  run application suites unless the edit changes an executable contract.
- Reuse existing fixtures and journeys for compatible assertions. New pipelines
  add source/mapping evidence and justified wiring coverage; they do not repeat
  shared lifecycle, recovery, ownership or publication test matrices.
- Run focused checks while iterating. Run a full suite when explicitly required
  or when the affected scope justifies it, after composing the relevant changes.
- Once sufficient checks pass, stop testing and complete the task. Repeat or
  broaden checks only for changed relevant inputs, a failure, a concrete unresolved
  risk, or an explicit requirement. Documentation or evidence bookkeeping alone
  does not invalidate passing runtime checks.
- If verification becomes unexpectedly slow, inspect the cause and remove
  redundant work before rerunning. Do not create new test infrastructure, audits
  or evidence paperwork merely to justify the time already spent.
- Preserve required safety and native-boundary coverage. Report what ran and any
  remaining limits accurately; never claim a skipped check passed.
