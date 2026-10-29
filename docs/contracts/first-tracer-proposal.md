# First tracer: contract proposal for S0

Status: draft for owner decision; not an accepted S0 output or implemented API.
Prepared 2026-10-29 under the request to execute the pipeline plan. This packet
makes D1/D2/D3/D5/D6 concrete for the local CloudTrail tracer only. It does not
change the source spec, settle later source/provider contracts, or claim S1 works.

Authority: [S0 and S1](../../../batz-specs/docs/specs/01-dfir-log-pipeline/pipeline-plan.md)
(the sibling repository link is informational; the product must not search peers).
Source plan SHA-256: 6ccb5b5afa0e4f8d6f571df178d06a706683e8922a767f18702e8fc305ef69c3.
Source spec SHA-256: 6cb30a343990eb6cc9a2f785d09f6b71a4831123453b844b77a473c985029ba4.

This is the decision packet, not yet the complete schemas/common Go types/locks
listed as S0 outputs. Once these decisions are accepted, complete those exact
contracts before starting S1. Do not treat examples below as executable assets.

```text
+-------------------+  proposed choices  +------------------+  accepted contracts  +-----------+
| S0 decision draft | =================> | Owner acceptance | ==================> | S1 tracer |
+-------------------+                    +------------------+                     +-----------+
```

## Decisions requested

Accept the recommendations below as one first-tracer contract, or name changes.
Runtime installation remains a distinct choice: the S1 setup would download and
verify project-local Go/Python/native dependencies, without system-wide updates.
No cloud credentials are needed for this tracer. The remainder of the plan stays
conditional on its own starts; accepting this packet does not settle S5 resources.

| Gate | Recommendation | Practical consequence |
| --- | --- | --- |
| D1 | Candidate Go 1.27.3, Bento 1.22.0, CPython 3.12.12, SQLMesh 0.236.4, DuckDB 1.5.4; built-in local JSON/Parquet functionality only initially. | Dependency metadata is feasible, but the composed stack is unqualified. Resolve full transitive locks and verify artifact hashes before execution. |
| D2 | ECS 9.5.0; profile `batz-investigation/v0alpha1`; nested fields below; exact supplied timestamp retained. | A narrow S3 GetObject mapping, with explicit unsupported coverage for other valid events. No claim of general CloudTrail/ECS conformance. |
| D3 | One explicit regular uncompressed CloudTrail Records JSON file per S1 invocation. Snapshot the selected bytes; bind identity to locator plus SHA-256. | No directory recursion, glob or implicit exclusion. Unreadable/changed-during-read input fails acquisition. Later slices add their selected source behavior. |
| D5 | `batz/v0alpha1` document discriminator; `--strict` for execution; exit scheme and SQL names below. | New public contracts require acceptance. Unsupported versions fail without altering existing evidence. |
| D6 | The finite first-fixture budgets below, explicitly disclosed in run reports. | These are proposed guardrails, not measured scale claims or permanent production defaults. |

Candidate versions are an initial qualification set, not a claim of latest versions
or compatibility. Bento's pinned module declares Go 1.27.1; installed Go 1.26.3 is
too old for that candidate. SQLMesh 0.236.4 declares Python >=3.9 and DuckDB <2.
DuckDB 1.5.4 preserves the version used in the historical narrow evidence, without
promoting that evidence to SQLMesh or nano fidelity. Artifact hashes, SQLite driver
pin, all transitive dependencies and compiled plugin inventory still need a lock;
the proposed location is `runtime/dependencies.lock.json`, accompanied by Go's
`go.mod`/`go.sum` and Python's fully resolved hash-locked requirements.
No fabricated checksums or empty lockfile counts as completion.

Sources: [Bento module](https://raw.githubusercontent.com/warpstreamlabs/bento/v1.22.0/go.mod),
[Go releases](https://go.dev/dl/),
[Python release](https://www.python.org/downloads/release/python-31212/),
[SQLMesh metadata](https://raw.githubusercontent.com/SQLMesh/sqlmesh/v0.236.4/pyproject.toml),
[DuckDB release](https://pypi.org/project/duckdb/1.5.4/),
[ECS release](https://github.com/elastic/ecs/releases/tag/v9.5.0).

## Proposed first-fixture profile

The example is synthetic, from the existing native Records fixture in the specs.
All nested groups below are typed STRUCT columns; category/type are VARCHAR lists.
Absent optional source values produce SQL NULL, never empty strings or invented
identities. Empty category/type lists are invalid for a mapped row. Unknown source
fields survive in retained parsed payload JSON. JSON text is identified as JSON
text rather than presented as nested canonical columns.

| Query field | DuckDB logical type | Meaning / first fixture expectation |
| --- | --- | --- |
| `"@timestamp"` | TIMESTAMP_NS, nullable | UTC occurrence instant; `2026-10-25T12:00:00Z`. Missing remains NULL. |
| `ecs.version` | VARCHAR, required | `9.5.0` |
| `event.kind` | VARCHAR, required | `event` |
| `event.category` | VARCHAR[], required | `['web']` for the API read |
| `event.type` | VARCHAR[], required | `['access']` |
| `event.action` | VARCHAR, required | `object-read`, the Batz action for this selected mapping |
| `event.outcome` | VARCHAR, required | `failure` when errorCode is present; otherwise `success` for this mapped API record |
| `event.provider` | VARCHAR, required | `s3.amazonaws.com` |
| `event.id` | VARCHAR, nullable | Original eventID, never an observation ID |
| `cloud.provider` | VARCHAR, required | `aws` |
| `cloud.region` | VARCHAR, nullable | Original awsRegion |
| `cloud.account.id` | VARCHAR, nullable | recipientAccountId only; no fallback to actor account |
| `user.id` | VARCHAR, nullable | IAMUser actor ARN when actually supplied |
| `source.ip` | VARCHAR, nullable | Validated source IP, `192.0.2.10`; non-IP caller stays in payload/extension |
| `batz.actor_account_id` | VARCHAR, nullable | userIdentity.accountId; distinct from recipient account |
| `batz.target.bucket`, `batz.target.key` | VARCHAR, nullable | Exact request bucket/key, no URL or resource ARN invented |
| `batz.time.original` | VARCHAR, nullable | Original eventTime including offset/digits |
| `batz.time.fraction_digits` | INTEGER, nullable | 0 for the first fixture; source precision indicator, not accuracy |
| `batz.time.offset` | VARCHAR, nullable | Original offset, including `Z` |
| `batz.time.uncertainty` | VARCHAR, required | `unspecified` unless a future source contract supplies evidence |
| `batz.profile` | VARCHAR, required | `batz-investigation/v0alpha1` |
| `batz.origins` | STRUCT[], required, nonempty | Observation IDs and exact parsed revision/file/row references |
| `batz.source_payload` | VARCHAR, required | Complete reconstructed JSON record; not claimed to be original raw bytes |

Mapping support initially requires eventSource=s3.amazonaws.com, eventName=GetObject,
eventType=AwsApiCall and userIdentity.type=IAMUser. Other valid source events remain
parsed/staging and count as unsupported. For a supported record, an invalid supplied
timestamp/type is a mapping reject, not silently unsupported. Missing occurrence
time remains explicit and does not substitute ingestion time. Source timestamps
outside the proposed nanosecond encoding range, or more precision than representable,
must stop acceptance for an owner tradeoff rather than truncate. S1 qualifies the
actual SQLMesh/Parquet/TIMESTAMP_NS path with distinct nano instants and a coarse
value; representation padding must not change fraction_digits.

The sample omits recipientAccountId, so cloud.account.id is NULL while
batz.actor_account_id is 111122223333. The actor and target stay distinct.
The complete source input is retained alongside its digest; reconstructed record
JSON has separate representation metadata. Source unknown fields are not rejected
merely because this compact profile does not map them.

[ECS event definitions](https://raw.githubusercontent.com/elastic/ecs/v9.5.0/schemas/event.yml)
supply classification conventions; the detailed mapping is this proposed Batz
contract. [CloudTrail record definitions](https://docs.aws.amazon.com/awscloudtrail/latest/userguide/cloudtrail-event-reference-record-contents.html)
distinguish the source fields. Exact field/null/semantic assertions must be reviewed
fixtures before implementation, including a same-type wrong actor/action/instant.

## CLI and manifest examples

Proposed direct run (paths are caller-relative; execution workspace is explicit):

```sh
batz run pipelines/aws-cloudtrail.yaml --route local_raw \
  --workspace /tmp/batz-demo --binding-id demo --input path=fixtures/cloudtrail/api-read.json
batz explain pipelines/aws-cloudtrail.yaml --level 1 --route local_raw
```

The CLI `--input key=value` form splits only on its first equals sign and can repeat
for distinct keys; duplicate keys are errors. No shell interpolation by Batz.
An omitted binding ID generates and reports a new opaque ID. Stable reuse requires
the same definition/version/route/source selection; mismatches fail preflight.
S1 exposes only local_raw. Later slices add their routes rather than publishing
nonfunctional assets under the historical mock names.

Proposed equivalent saved selection (full parser belongs to its planned slice):

```yaml
apiVersion: batz/v0alpha1
kind: PipelineBinding
id: demo
definition: repo://pipelines/aws-cloudtrail.yaml
version: 1
route: local_raw
input:
  path: repo://fixtures/cloudtrail/api-read.json
```

Workspace stays explicit through `--workspace`. Definitions retain the spec's
routes/input/output structure and shared normalization.project. Reserved child
handoff uses BATZ_STAGE_DIR, BATZ_RUN_ID, BATZ_BINDING_ID, BATZ_BATCH_ID and
BATZ_ATTEMPT_ID; optional BATZ_CASE_ID. Source parameters cannot replace these.
The local source parameter uses BATZ_SOURCE_PATH. No credential slots are needed.

Proposed later spellings, reserved here for owner review, not S1 completion claims:

```sh
batz retry --workspace /tmp/batz-demo --run-id <run-id> --stage normalize
batz status --workspace /tmp/batz-demo --run-id <run-id> --format json
batz run <definition> --route <route> --workspace <path> --strict ...
```

| Exit | Meaning |
| --- | --- |
| 0 | Requested command succeeded, including published partial by default or successful status retrieval of a failed run |
| 1 | Acquisition/model/publication/execution failure, or mixed-binding operational failure |
| 2 | Invalid document/version/selection or endpoint authorization |
| 3 | Required capability/dependency unavailable |
| 4 | Integrity or required assertion failure |
| 5 | Busy workspace |
| 6 | Strict command rejected a durably published partial result |
| 130 | Cancellation |

Machine diagnostics use a category, stable code, field/source location and message;
category distinguishes authorization from invalid input even when both exit 2.
A mixed command selects a deterministic priority: cancellation, integrity, invalid/
authorization, missing capability, busy, execution, strict partial. It retains
every binding outcome. Strictness never rolls back publication.

## Wire and reader contract proposed for completion after acceptance

All documents use apiVersion=batz/v0alpha1 and an explicit kind. Receipt kind is
CollectionReceipt; portable inventory kind is PublishedInventory; report kind is
RunReport. Unknown/duplicate fields and unsupported versions fail before mutation.
IDs are nonempty opaque strings, never paths. File references contain artifact-
relative path, SHA-256, byte count and row count; numeric counts are nonnegative
signed 64-bit integers. Digests are lowercase 64-character hexadecimal strings.

The receipt names binding/run/batch/attempt, frozen selected input identity, exact
closed parsed/reject/raw files, parse counts, complete status and nullable proposed
continuation. Clean child exit and verification of actual files remain mandatory.
A receipt is untrusted wire data, never a constructible VerifiedReceipt capability.

Input identity for this local tracer is canonical source locator plus a SHA-256
of the retained source bytes. Modification during snapshotting fails acquisition;
the collector consumes only that snapshot. Record ordinal is zero-based within
Records. Observation identity includes binding, batch, input digest and ordinal;
a new explicit replay has a new batch. Retrying publication retains the batch.
Event identity is separate; S4 still settles reliable source scope/correction rules.
S1 does not claim cross-batch reconciliation qualification.

RunReport separates acquisition completion, processing quality, mapping coverage,
command outcome/exit, strictness, parsed/current normalized revisions, and named
boundary counts. It records all input/content/model/dependency identities, limits,
nonsecret resolution/origins and diagnostics. Never serialize resolved secrets.

The portable inventory lists exact datasets/files, source/profile/model identity,
input membership digest, parsed origins, rejects, counts and immutable revision.
Artifact data references must stay under the explicitly selected artifact root:
reject absolute paths, traversal and symlink escape. Original source locators
are provenance only, never inputs to reader resolution after relocation.
Checksums verify the exact files before catalog creation.

Proposed ordinary SQL interface:

```sql
ATTACH '/tmp/batz-demo/.batz/readers/<revision>.duckdb' AS a (READ_ONLY);
SELECT event.action, user.id, "@timestamp", batz.origins
FROM a."demo".api_activity;
SELECT * FROM a."demo".parsed;
SELECT * FROM a."demo".source_staging;
SELECT * FROM a.batz_inventory.membership;
```

Each catalog exposes only its exact committed revision. Report the catalog path
and revision on successful publication. Publish B into a different immutable file;
an existing connection can ATTACH it AS b while a remains attached. Dataset names
remain logical binding.dataset addresses; model IDs are never guessed table names.
Membership maps each binding/dataset to revision, relative files and input digest.
S8/S9 specify destination catalog setup against the portable inventory. Explicit
materialized reader tables remain unsupported initially.

Complete schemas and Go seam types must define every serialized field, nullability,
error conversion and ownership before S1. The above establishes decisions without
inventing incompatible per-module copies. Schema generation, parser implementation
and native qualification have not occurred in this draft.

## First-fixture budgets proposed

Limits are per run and include descendants: one collector, one model job; maximum
one input file, 16 MiB source bytes, 10,000 Records elements, 120 seconds collection,
300 seconds modeling, 1 GiB collector process-tree memory and 2 GiB model-tree memory,
one DuckDB thread and 1 GiB private scratch/output bytes. Each is finite and recorded.
Timeout/resource exhaustion is an operational failure, never record-level partial.
No automatic retry in S1. Installation/build time is separate from runtime budgets.

These are test-fixture starting values. Qualify how each is enforced on native
macOS before claiming that bound. Missing enforcement cannot be reported as passing.
Do not apply these values as S2's offline sandbox policy or S3/S8 production defaults;
those slices retain their own affected D6 decisions.

## Concrete positive and negative cases

[decision-examples.json](decision-examples.json) contains complete tiny source
documents and expected decision outcomes; they are proposals, not test results.

- Valid GetObject: one retained observation, one mapped row, complete quality,
  zero rejects, actor ARN and object-read action, second-granular source time.
- Valid unsupported PutObject: retained parsed/staging observation, zero mapped,
  one unsupported, complete quality; neither empty coverage nor unsupported is a reject.
- Valid GetObject with malformed eventTime: retained reject and reason, partial
  processing after required checks; default exit 0, strict exit 6.
- Records=[]: explicit completed empty inventory, checked empty normalized output.
- CSV/reformatted array or unreadable input: batch failure, no parsed commit/cursor.
- apiVersion=batz/v9: invalid version exit 2, no collector, prior evidence unchanged.
- Receipt for the wrong attempt or ../escape.parquet: integrity failure, no commit.
- Existing model interval plus new committed input: new exact membership must be
  evaluated; stale export/digest cannot replace the prior normalized pointer.
- Two synthetic timestamps one nanosecond apart: both distinct after publication
  and ordinary SQL; a deliberately wrong expected instant must fail the real oracle.

## Deferred and evidence limits

Provider reference/authentication/delivery, R2 grants, general CloudTrail event
scope and authoritative corrections, GCP/Workspace details, complete explain/CI
schemas, sandbox support and cross-platform qualification remain in their planned
later slices. No mock is promoted to a supported route. S1's own publication,
ownership, build freshness and Python-refusal invariants still apply in full.

Observed host: Darwin arm64; Go 1.26.3; system Python 3.9.6; system Python package
metadata has neither SQLMesh nor DuckDB. This is not a machine-wide inventory.
uv's installed-Python listing failed on cache access and is inconclusive.
No runtime dependency was installed. No native product test ran.

Review strategy: focused-postcheck for this draft; public-behavior critical-tdd
for later implementation. The plan's fresh whole-change review remains due after
composition. This unaccepted proposal is not a passed implementation review or S0-A.
