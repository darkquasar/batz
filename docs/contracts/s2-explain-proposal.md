# S2 contract proposal: explain the same fixture offline

Status: proposed for owner acceptance, not an implemented capability. S1's local
source-to-query path and static overview are the predecessor. The plan's S2
external starts explicitly require the next report/config contract and projection
policy before public parser/CLI work. This packet makes that choice concrete.
It grants no live-source access, credentials, provisioning or publication.

## User-visible result

```text
+---------------+  local fixture  +-------------------+  inspect stages  +----------------+
| Explain CLI   | ==============> | Isolated preview  | ===============> | Values + types |
+---------------+                 +-------------------+                  +----------------+
```

Recommend these cumulative levels:

- Level 1: the existing route/asset overview, plus named outputs and unresolved values.
- Level 2: decoding, parsed publication, SQL staging, model dependencies and lineage.
- Level 3: actual sample values and types at each supported stage, with original time
  precision. Label every result declared, inferred, sample-executed or unavailable.

Keep `batz explain <pipeline.yaml> --level 1|2|3 --route NAME --format text|json
--strict`, and add the spec's `--example ID_OR_PATH`. A file path needs an explicit
route association; its extension never selects a route. No compatible example or
no qualified sandbox yields known declarations plus an unavailable explanation.

Register the existing synthetic `api-read` example under `explain.examples`, as a
list of `{id, route, input, sha256, expectations}`. It belongs only to `local_raw`.
`input` and `expectations` are declaring-file/repo paths. Example IDs are unique;
expected hashes and expectations are reviewed inputs, never regenerated from a
failing run. This adds no remote/Velociraptor support.

For this example, the reviewed semantic expectations remain S1's real results:
`event.action=object-read`, actor ARN
`arn:aws:iam::111122223333:user/example-responder`, occurrence time
`2026-10-25T12:00:00Z`, fraction_digits=0, source.ip=192.0.2.10, and the preserved
unmapped_example value `kept`. These are proposed preview expectations based on
S1 evidence, not a claim that S2 preview has executed.

## Structured report

Continue the explicit `batz/v0alpha1` alpha family with a documented contract
amendment. Unknown versions/fields fail rather than being reinterpreted. Preserve
S1 fields and add these named sections; complete closed schemas and positive/negative
examples after acceptance, before parser implementation.

| Section | Proposed contents |
| --- | --- |
| pipeline/version/level/project/routes | Selected declaration identity and deterministic route order |
| assets | Loaded native config, model, example and expectation SHA-256 identities |
| resolution | Effective nonsecret values and original source/field/layer; unresolved paths stay symbolic |
| stages | Ordered stage ID, route, kind, input/dependency IDs, native references, outputs, basis and reason |
| structures | Stage/dataset, native type, nullable status, recursive fields, encoding and basis |
| samples | Example ID/route/hash, bounded records/counts/rejects, supported/unsupported coverage, observed and unobserved branches |
| diagnostics | Existing category/code/field/source/message shape |
| runtime | Elapsed time, RAM measurements and scratch-only runtime details; excluded from semantic comparison |

Logical stages are source document, decoded record, parsed envelope, shared SQL
staging and each named normalized output. A JSON payload stored in VARCHAR is
explicitly JSON text; nested canonical STRUCT/LIST columns are shown as nested
columns. Native timestamps render losslessly as ISO strings with their native
precision, alongside their type and original source granularity. Integers outside
JSON-safe consumer precision render as decimal strings with an explicit native
integer type. Missing values remain null; unknown shapes remain unavailable.

Default semantic comparison includes declarations, asset/example hashes, structures,
source-derived values, counts, diagnostics and branch coverage. Runtime-generated
IDs/nonces, acquisition clock values, absolute scratch paths, durations and RAM
samples are separated into runtime metadata, with explicit references from stage
rows where necessary. They are not rewritten to look like source facts. Two
identical previews must have equal stable sections. S1's second-granular fixture
does not claim coverage for fractional time, rejects, empty input or other branches.

## Configuration conventions for this slice

Use optional `<project>/workspace.yaml` and only the explicitly selected
`<workspace>/workspace.yaml`. Never search ancestor workspaces, nearby cases or
peer repositories. Keep the six-layer order and merge/null/path semantics already
settled by PIPE-004/005; no new precedence choice is proposed.

Add optional typed `default` values to parameter declarations for the route-default
layer. Only `storage`, `normalization`, `credentials` and scoped
`pipeline_defaults.<source>.<route>` are shared sections. Bindings keep flat
`id`, `definition`, `version`, `route`, `input`, `credential_aliases`; a saved binding
may also explicitly select `workspace` and shared sections. A `Case` has `id`,
optional `workspace`, shared sections and a `pipelines` list of binding entries.
A `Workspace` has the four shared sections. All use versioned closed schemas;
blank YAML values fail, while explicit null deletes before required validation.

Recommend these selection forms in addition to S1's direct run:

```sh
batz run --binding path/to/binding.yaml --workspace /tmp/example
batz run --case path/to/case.yaml --binding-id selected-binding
batz explain pipelines/aws-cloudtrail.yaml --binding path/to/binding.yaml --level 3
batz explain pipelines/aws-cloudtrail.yaml --case path/to/case.yaml --binding-id selected-binding
```

`--binding` and `--case` are mutually exclusive. The case form selects one explicit
binding in S2; multi-binding scheduling and case-wide publication remain S8.
A positional definition with binding/case context must identify the same definition;
CLI `--input` and `--route` override resolved values under the settled precedence.
Run still requires an explicit workspace after resolution. `--interactive` opts
into missing-only prompts; unattended commands aggregate static diagnostics and
start no collector. String-list CLI values use JSON arrays and native delivery is
compact JSON. Timestamp delivery preserves the supplied RFC3339 text.

Historical provider descriptors remain static precedence examples. They cannot
cause secret lookup or execution support for aws_profile/file/aws_vault. Exact
1Password/environment delivery and endpoint authorization remain S5. Storage paths
and provider descriptions are resolved and reported honestly; a selection requiring
an execution capability not yet qualified fails explicitly rather than ignoring it.

## Local preview policy

Use the already installed, checksum-verified native Bento/SQLMesh/DuckDB stack.
Allow only the selected fixture, admitted declarative project and pinned runtime
as read inputs; use a private scratch root for all writes. The preview must deny
network access, secret lookup, source fetching, dependency/extension installation,
project Python and production workspace/state writes on the actual children.
An unenforceable host/adapter is unavailable before launch, with no fallback run.
Native macOS is the first qualification target; Linux/container proof remains S9.

Reuse the selected local-development budgets: one input, 16 MiB, 10,000 records,
120s collection, 300s modeling, one DuckDB thread, one collector/model at a time,
and a sampled 1 GiB scratch stop threshold with possible brief overshoot.
Honor the owner's current RAM instruction: monitor average/peak usage with no Batz
RAM cutoff. This is an explicit local-development exception to PIPE-025's original
finite-RAM requirement, not a claim that RAM isolation has been enforced. Actual
filesystem/network isolation remains mandatory. Production resource defaults are
still later decisions.

Default explain returns 0 for known declarations with unavailable optional preview;
`--strict` returns 3 when requested structures/projection cannot be established.
Invalid documents, unknown routes or incompatible explicit examples return 2.
An attempted native projection that fails operationally returns 1; integrity or
required semantic assertion failure returns 4. Missing live credentials alone is
not an offline-preview failure. No explain path creates production run/revision
state or silently downloads assets.

Acceptance of this proposal authorizes completing its exact schemas/examples and
implementing S2 under the existing serial/TDD/review plan. It does not claim that
sandbox enforcement works before native positive/negative qualification.
