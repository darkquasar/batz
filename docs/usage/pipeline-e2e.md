# Pipeline native contract checks

Routine pipeline contributions use native model/adapter contracts. The former
per-source Go application E2E blueprint is superseded by S3.2-CI; shared synthetic
application proof owns run coordination, publication and reader behavior.
Follow [contributing](../../contributing.md) and [ADR-001](../adr/0001-test-at-the-cheapest-sufficient-layer.md).
The filename is retained so existing contributor links still resolve.

```text
+----------------+  acquire  +----------------+  native SQL  +-------------------+
| Source fixture | ========> | Actual adapter | ===========> | Declared roots    |
+----------------+           +----------------+              +-------------------+
                                                                      | validate
                                                                      v
                                                            +-------------------+
                                                            | Profile + origins |
                                                            | checks + expected |
                                                            +-------------------+
```

| Criterion | Required evidence |
| --- | --- |
| Scope | Source contract, manifest/route, small fixture with provenance/checksum and independent expectations. The first explain example is the representative contract fixture. |
| Adapter | Real collector built from this checkout, source framing, parsed payload JSON fidelity and malformed-input refusal. |
| Native integration | Actual parsed output/schema feeds staging, evidence, required-check and output roots with their dependencies. Native audits and required checks pass. |
| Integrity | Production contract/profile/build and exact origin/accounting validators run on the native result. No mirrored profile schema. Fan-out and multiple datasets retain exact mapped membership. |
| Source meaning | Native tests and representative expectations distinguish action, actor, outcome, source time interpretation, payload and origins as applicable. Optional identity remains optional. |
| Representation | Native type/epoch queries execute on actual outputs. Explicit native fixture types and source-time cases preserve offset, fractional, missing and unsupported values. |
| Execution | Selected test IDs match executed successes exactly. Missing, empty, failed, omitted or skipped selected checks fail. |

```sh
ci/test fast
ci/test models aws-cloudtrail --mode native
ci/test models aws-cloudtrail --mode adapters
```

The representative CloudTrail expectations assert the exact nanosecond timestamp
through native JSON serialization for UTC and offset forms of the same instant.
Source-native tests separately retain distinct 1 ns/2 ns output expectations.

Native mode uses explicit model fixtures independently of adapter schema discovery.
Adapter mode uses one loaded native context for the selected contract and production
validation functions. It creates no Batz workspace, inventory commit, published
revision or reader process. `ci/test models aws-cloudtrail` composes both modes.

The [CloudTrail manifest](../../pipelines/aws-cloudtrail/pipeline.yaml),
[representative fixture](../../pipelines/aws-cloudtrail/fixtures/wiring.json),
[expectations](../../pipelines/aws-cloudtrail/fixtures/wiring.expected.json) and
[native tests](../../tests/aws-cloudtrail/test_mapping.yaml) are ordinary source
examples. Additional explain examples are not automatically contract-evaluated;
explicit preview executes their declared expectations.

One [shared synthetic journey](../../internal/app/app_runtime_test.go) reuses real
collector/model/publication/reader executions for representative app wiring and
model-only recovery. Separate focused real-boundary tests retain cancellation,
ownership, sandbox, preview, strictness and replay guarantees. They load no vendor
models or samples. A different source model graph requires its native contract,
without another application journey. A targeted core interaction belongs in shared
synthetic proof. Specifically promised remote/platform qualifications remain scoped
obligations; no local native fixture qualifies them.

Report the route, fixture, assertions, actual commands/results and limitations in
the normal PR. Timings exclude no required work: setup/cold builds and hosted
queue/job/PR duration are separate observations. See the [CI guide](ci.md).

The repository synthetic fixture currently keeps its models self-contained; the
shared journey also constructs an invented shared ancestor in its private fixture.
When adding a real shared-SQL/global-content dependency to that fixture, include
its consumed content in the fixture-copy helper and verify that journey. The
selector already recognizes that dependency via native base/head impact; it does
not copy source files.
