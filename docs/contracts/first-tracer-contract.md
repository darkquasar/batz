# Accepted first-tracer contracts

The owner replied "ok cool I agree" after the S0 acceptance request and clarification
of the synthetic fixture path. Continue with the first-tracer proposal and
checksum-verified project-local dependency setup. The preserved
[first-tracer proposal](first-tracer-proposal.md) and [decision examples](decision-examples.json)
record the reviewed choices. This document supplies their exact S0 wire/type detail.
It does not claim S1 behavior, grant cloud access, or settle later slice decisions.

Schema version: batz/v0alpha1. Profile: batz-investigation/v0alpha1, ECS 9.5.0.
The alpha policy permits explicit future revisions; unsupported versions must
fail before mutation and never silently reinterpret retained evidence.

## Documents, shared types and ownership

[Wire types](../../internal/contracts/wire.go) are data-only Go structures.
[Common schema](../../schemas/v0alpha1/common.schema.json) defines shared fields.
Every schema is JSON Schema 2020-12. The batz.invalid IDs are offline identifiers
in the reserved .invalid domain; validation must register local schemas, not fetch
them. YAML source documents use the equivalent JSON data model. Duplicate keys
must be rejected by decoding before schema validation; schema validation alone
cannot detect a duplicate already discarded by a decoder.

| Wire kind | Exact schema | Typed boundary |
| --- | --- | --- |
| SourcePipeline | [source-pipeline](../../schemas/v0alpha1/source-pipeline.schema.json) | Resolve -> SourcePipeline |
| PipelineBinding | [pipeline-binding](../../schemas/v0alpha1/pipeline-binding.schema.json) | Resolve -> PipelineBinding; saved-binding execution comes in S2 |
| CollectionReceipt | [collection-receipt](../../schemas/v0alpha1/collection-receipt.schema.json) | Collect output -> VerifyAndPlace input |
| PublishedInventory | [published-inventory](../../schemas/v0alpha1/published-inventory.schema.json) | Committed normalized publication -> PrepareCatalog/reader |
| RunReport | [run-report](../../schemas/v0alpha1/run-report.schema.json) | Run outcome -> reporting |
| ExplainReport | [explain-report](../../schemas/v0alpha1/explain-report.schema.json) | Explain level 1 -> deterministic reporting |

All listed object fields are required, including explicit null for absent optional
facts. Empty collections serialize as [] or {}, never null. Unknown keys fail.
Manifest field names preserve apiVersion camel case; other wire fields use snake
case. Source YAML field names match JSON tags. The initial route has no credential
slots; later source/provider schemas remain pending before their own authoring.
The initial SourcePipeline normalization.project selects the shared declarative
SQL project. Additional shared default/override sections are S2 amendments.

Identifiers contain 1–128 ASCII alphanumeric, underscore or hyphen characters,
beginning with an alphanumeric. Generate random opaque IDs and never use an
external ID directly as a filesystem path. IDs are not interchangeable: binding
is stable selection scope; run is an invocation; batch is one finite acquisition;
attempt is one try; revision is an immutable published selection. File paths are
artifact-relative slash-separated locations; reject NUL/control characters,
absolute/drive paths, backslashes, parent traversal and symlink escape. Lexical
schema checks supplement rather than replace filesystem confinement.

Hashes are lowercase SHA-256 hex. Counts, bytes and rows are nonnegative int64.
FileRef.rows=0 for raw source/catalog files, whose row cardinality is in datasets.
An input's id is SHA-256 of UTF-8 JSON [canonical_locator, content_sha256], compact
encoding with no trailing newline; it scopes a local content fingerprint to its
source. Locator remains provenance after relocation. Observation IDs use SHA-256
of compact JSON [binding_id, batch_id, input_id, zero_based_record_ordinal].
Cryptographic hashes identify bytes/scopes; they are never universal event IDs.

## Verification and publication seam

```text
+--------------+  verify/place  +-----------------+  commit  +------------------+
| Receipt data | =============> | VerifiedReceipt | ======> | Committed inputs |
+--------------+                +-----------------+         +------------------+
```

CollectionReceipt complete=true is only a claim by the child. VerifyAndPlace must
also establish clean finite exit, expected identity/input set, closed actual
Parquet files/schema, exact file digests/rows, counts and source evidence. Place
durable immutable files before the SQLite inventory/status/cursor commit. Do not
construct trusted state from a caller-supplied struct or from a receipt file alone.

The publication module exclusively creates its private VerifiedReceipt and
CommittedInventory concrete values. It returns defensive copies at read-only
interfaces. No public constructor/setter in contracts may mint either capability.
The model module consumes only a committed inventory and supplies a private
BuiltRevision with input revision/digest, content identity, attempt, actual
evaluation/export evidence and required check results. PublishNormalized rechecks
that evidence and the actual outputs against its selected committed input before
changing the pointer. Empty builds need actual checked evaluation too.

Membership digest is SHA-256 of canonical JSON containing binding_id, input
revision, and files sorted by relative path with path/SHA-256/bytes/rows. Canonical
JSON means object keys lexicographically sorted, arrays in declared order, integer
decimal notation and UTF-8 strings escaped by JSON rules without whitespace.
The implementation must share one encoder, document any escaping normalization
and test digest agreement at its real producer/consumer seam. Input file
membership is not discovered by an open-root glob.

Parse counts: accepted+rejected equals the number of observed Records elements;
unsupported and excluded are zero at this boundary. Normalize counts:
accepted+rejected+unsupported equals parsed observations processed by this narrow
mapping. Parse rejects remain separately counted, durable and visible in the run.
Overall partial means any parse/normalize rejects; unsupported does not imply
partial. An unreadable/truncated acquisition or invalid receipt never becomes a
record reject. source_files retain source bytes; reject_files retain undecodable
or unmappable evidence plus reason; each normalized origin resolves parsed
revision/file/row. An invalid count equation blocks the relevant commit.

PublishedInventory is an export of committed state. Its datasets include parsed,
source_staging and api_activity exact file lists, with schema and optional model
ID. A file list may be empty only when the associated checked dataset is empty.
Dataset rows must equal summed member rows; duplicate file membership and duplicate
dataset names fail validation. The catalog reference names a newly closed immutable
catalog. It is replaceable on relocation, never used as the only portable data
inventory. Each destination catalog resolves only validated relative data paths
at the explicitly selected artifact root. Source locators are never reopened by
reader preparation.

## Reporting and error semantics

RunReport.outcome names the stage/result and is independent of acquisition_complete,
quality, coverage, strict and exit. quality is null before successful normalized
publication. parsed_revision/current normalized_revision can retain earlier
committed facts after later failure; report the exact selected identities.
Partial strict publication keeps its new normalized revision and returns exit 6.
Successful status retrieval has command exit 0 while the stored run.exit remains
the original result. CLI preflight failures that precede run/identity creation
emit diagnostics without fabricating a persisted RunReport.

Diagnostic fields category/code/field/source/message are always present; field
and source can be empty when there is no document location. Codes are stable
machine-readable identifiers; messages are descriptive and must not contain
credentials. Proposed initial codes: invalid-version, invalid-field, unknown-route,
missing-workspace, missing-dependency, missing-asset, unsupported-content,
unsupported-shape, reserved-handoff, dependency-integrity, receipt-integrity,
build-integrity, required-check, workspace-busy, cancelled, acquisition-failed,
normalization-failed and publication-failed. Categories and exits follow the
accepted proposal. Location/origin fields always describe the declaring document
or cli, never a merged value with a fabricated origin.

Level-1 ExplainReport exposes ordered routes, declared engine/config/format/model
references, loaded asset hashes and diagnostics. It runs no collector, framework
import, vault, network request or installation. All routes sort by name; JSON
object keys/stable sections serialize deterministically. Deeper explain reports,
projection schemas and enforcement remain S2 work rather than successful stubs.

## Private request types and later seams

Resolve's Selection carries explicit project/workspace/definition/route, binding
ID, typed input values and strictness; each resolved value keeps a ValueOrigin.
CollectionRequest carries Identity, frozen InputRef and source FileRef, private
staging path and Limits. It passes generated BATZ_STAGE_DIR/BATZ_* identity values
plus a read-only nonsecret context document and the snapshotted BATZ_SOURCE_PATH
to the child. Source parameters cannot replace coordinator-owned values.

ModelRequest contains the pinned profile/content/dependency identities, private
attempt directory, isolated binding state and Limits. ReaderRevision is the
validated PublishedInventory plus the explicit local artifact root. RunRequest
combines resolved selection and strictness; RunResult is the RunReport plus
diagnostics when preflight failed. ExplainRequest contains selection, level and
strictness but never a SecretLease.

OwnerLease, VerifiedReceipt, CommittedInventory and BuiltRevision implementation
types belong to workspace/publication/model owners, not this serializable package.
The S1 implementation defines their minimal methods at real consumers under TDD;
the plan's invariants, identities and wire fields above govern them. No behavior
or trusted capability has been implemented in the data-only S0 Go package.
Authorize/Deliver and SecretLease/ApprovedEndpoint/ProviderRef remain S5 details;
no local fixture requires their implementation or an ambient identity lookup.

## Limits and qualification

The owner first accepted frequent checks with brief overshoot, then explicitly
amended RAM policy: "for now, you are allowed to use as much ram as needed, just
monitor it so we have an idea of averages". This supersedes the original collector
and model RAM maxima for S1. There is no Batz RAM cutoff and no added DuckDB 2 GB
cap. Framework/system memory management still applies.

Current Limits: one input, 16 MiB, 10,000 records, 120s collector, 300s model, one
DuckDB thread, one collection slot and one model slot. Scratch/output uses a 1 GiB
sampled stop threshold, checked before/after a child and on a nominal 100 ms timer.
Overshoot between checks is accepted; this is not a hard instantaneous disk quota.
Count private attempt files, current acquisition outputs and the model working/state
files, including logs. Retained outputs from earlier runs are outside this per-run
scratch measurement. Exceeding the threshold stops and settles the child group
and fails the stage before its publication. Coordinator-death recovery remains S3.

Documented alpha wire amendment: Limits.collector_memory_bytes and
Limits.model_memory_bytes now permit null; null means no Batz memory cutoff.
RunReport adds required resources, keyed by collection/model for started stages.
Each stage records average_rss_bytes, peak_rss_bytes, samples, unavailable_samples,
sample_interval_ms, wall_ms, scratch_peak_bytes and measurement_error. Average is
the arithmetic mean of successful active-process samples, not a time-weighted mean.
RSS is summed across the observed process group and descendants; shared pages can
be counted more than once. It excludes the Batz coordinator. Null average/peak
means no successful active sample, not zero usage. Unavailable samples and the last
measurement error are explicit; RAM measurement failure cannot invent a number.
This schema amendment is a documented change within the explicitly alpha contract;
pre-amendment example/evidence hashes remain historical, not current validation.

The new policy concerns S1 execution. It does not settle S2 offline isolation,
future production resource defaults, native Linux/container qualification or a
throughput/scale claim. Actual native checks, not schema validation, establish
the measured process behavior.

[Examples](examples/) use invented digests and paths to exercise document shapes;
they are not receipts for existing files. Focused S0 checks validate schemas and
positive/negative examples and compile the data-only types. S1 adds public-behavior
critical-tdd, real Bento/SQLMesh/DuckDB checks and the named boundary review.
