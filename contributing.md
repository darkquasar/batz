# Contributing data sources and pipelines

This is the baseline journey for adding a source integration. `okta-audit` below
is an illustrative pipeline name, not an implemented or qualified integration.
Source research and detailed mapping design belong to the contribution.

Start with the [specification](../specs/batz/01-dfir-log-pipeline/pipeline-spec.md),
the applicable [plan](../specs/batz/01-dfir-log-pipeline/pipeline-plan.md) slice,
and the [pipeline contract](docs/usage/pipeline-contract.md). Prepare the
[pinned tools](runtime/dependencies.lock.json); native lanes currently require
the qualified macOS ARM64 environment. See [README](README.md) for local/CI scope
and the external toolchain option.

```text
+----------------+  implement  +------------------+  verify  +----------------+
| Source contract| =========> | Pipeline content | =======> | Focused checks |
+----------------+             +------------------+          +----------------+
                                                                    | milestone
                                                                    v
                                                            +----------------+
                                                            | Review and PR  |
                                                            +----------------+
```

## 1. Define the source contract

Describe the accepted input format, supported events, output meaning, timestamp
semantics and limitations. Keep saved-file import and live acquisition scopes
explicit. New sources or transports need their applicable acceptance requirements.

Check whether existing framing handles the source's native format. Current
framing supports `records-json` and the synthetic `jsonl` probe. A different
format needs a focused adapter extension with independent fidelity checks;
preserve the source format instead of disguising it as another vendor's input.

## 2. Add pipeline-owned content

A typical contribution would add:

```text
pipelines/okta-audit.yaml
collector/configs/okta/local.yaml
normalization/okta/config.yaml
normalization/okta/models/*.sql
normalization/okta/tests/test_mapping.yaml
fixtures/okta/sample.json
fixtures/okta/sample.expected.json
internal/app/okta_audit_e2e_test.go
```

The `SourcePipeline` manifest declares its identity, version, project, framing,
route configuration, staging/evidence/check models and output model IDs. Declare
the representative source example, checksum, route and expectations too. The
model-test driver currently uses the first declared example for adapter proof.
Manifest discovery is automatic; there is no pipeline registry to edit.

Put source mapping and semantic checks in native SQLMesh SQL. Batz retains common
schema, identity, membership, lineage, accounting and publication checks. Use the
existing contract and small synthetic project under
[testdata](internal/app/testdata/synthetic/) as structural references.

Each pipeline currently owns a SQLMesh project shared by its routes, using the
same installed toolchain as other projects. This Batz boundary keeps source
loading and verification independent. It adds configuration duplication and makes
cross-source model dependencies less convenient. SQLMesh also supports shared
projects and model selection; project separation is an architectural choice.

## 3. Add small, meaningful examples

Use SQLMesh's [native unit tests](https://sqlmesh.readthedocs.io/en/latest/concepts/tests/)
with small input rows and explicit expected output rows. Cover the distinguishing
mapping behavior, meaningful malformed/unsupported cases and source-specific
time interpretation. Batch compatible cases into a native suite.

Expected values express the researched source meaning; review them alongside the
SQL in the normal PR. Blindly recording the current model's output can preserve
an existing mapping error. No separate fixture-review ceremony is required.

Also keep one small synthetic or safely anonymized source sample for actual
adapter/E2E verification and explain. Its checksum verifies the sample's bytes;
the expected results verify meaning. Generic type/count/lineage checks cannot
determine whether a vendor-specific actor or outcome was mapped correctly.

## 4. Iterate on the selected pipeline

Once the manifest and tests exist, use the pipeline's name:

```sh
ci/test models okta-audit
```

This runs its actual adapter and batched native model tests in private scratch.
Keep mapping iteration here; production skips contributor unit tests.

## 5. Prove the wiring once

Add a representative source-to-reader test in `internal/app/okta_audit_e2e_test.go`
with build tag `batz_okta_audit` and a test name beginning with `TestE2E`.
Hyphens in the manifest identity become underscores in the filename and tag.
The current authoring experience includes this small Go integration test.

```sh
ci/test fast
ci/test e2e okta-audit
```

Query the published output and verify the source's distinguishing values and
native types. Reuse the shared recovery, ownership, cancellation and publication
proof. Changes to shared machinery need affected shared checks, including
`ci/test app`; an ordinary mapping contribution does not duplicate that suite.

## 6. Qualify and review the contribution

Run final local composition at the milestone:

```sh
ci/test all-local
```

Include the supported scope, reviewed expectations, actual check results and
limitations in the PR. Follow the plan's milestone review gates, including the
Claude checkpoint; retain findings and their correction/disposition evidence.

The authored CI selector derives ownership from manifests at merge-base and
head. A pipeline-only change selects its model/adapter and E2E checks plus fast
contracts. Shared adapter/bridge changes broaden compatibility checks. CI calls
the same local commands; runner activation remains unverified.

Keep verification affordable: share lifecycle proof, batch source semantics,
and add native journeys only for distinguishing integration behavior. Use
standard Go and SQLMesh facilities without adding another assertion framework
or mutable fixture cache.
