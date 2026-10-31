# Pipeline contracts

SourcePipeline uses `apiVersion: batz/v0alpha2`. Earlier SourcePipeline versions
are refused before collection; retained reports, receipts and immutable published
data continue to use their existing `batz/v0alpha1` formats. The pipeline `version`
remains the selected content revision, distinct from the manifest format version.

Each pipeline declares its own `normalization.project`, shared by its routes.
`normalization.staging`, `evidence` and `checks` name native SQLMesh models.
Routes declare `input.framing` (`records-json` or the synthetic `jsonl` adapter)
and map individual output dataset names to native model IDs. SQLMesh validates
references before acquisition and owns dependencies, planning and application.
The JSONL fixture is an internal contract probe; it does not qualify GCP exports.

```text
+------------------+ declared models +-------------------+
| Pipeline manifest| ==============> | Native SQLMesh    |
+------------------+                 +-------------------+
                                            | evaluated common evidence
                                            v
                                    +-------------------+
                                    | Batz publication  |
                                    +-------------------+
```

Staging preserves every parsed observation's common envelope and adds
`parsed_revision`, `parsed_file`, `parsed_row`, `mapping_status` and
`mapping_reject_reason`. Mapping status is `mapped`, `rejected`, `unsupported`, or
`unavailable`; an unavailable representation blocks publication rather than
becoming a record reject. Source adapters independently verify framing, ordinals
and source payload fidelity.

The evidence model returns exactly one row containing `input_revision`,
`input_digest`, `attempt_id`, `evaluation_nonce`, `observations`, `mapped`,
`unsupported` and `rejected`. The checks model returns exactly one row containing
nonnegative integer `assertion_failures` and `capability_failures`. Missing or
failed assertions return exit 4; unavailable representation returns exit 3.
Pipeline SQL owns vendor semantics; native model tests and reviewed expectations
provide independent semantic evidence.

Output models use the nested investigation profile, including `TIMESTAMP_NS`
`@timestamp`, nested `ecs`, `event` and `batz`, exact build identity and source
payload/origin references. Batz verifies the common schema, exact staging and
mapped-origin membership, accounting and evaluated build evidence. Distinct
outputs may share observations. The current observation contract uses one origin
per output row; reconciliation and multi-origin rows remain S4 work.

Production skips contributor unit tests. Production identity includes the selected
resolved route contract, native config, runtime/coordinator identities and copied
production project content. Explain examples and project tests do not invalidate
an existing normalized revision. Executed config/model/audit/macro/seed edits do.
All SQLMesh test contexts, caches and state live in private temporary directories.
Static explain reports declared boundaries without guessing SQL filenames or
claiming observed schemas; native projection supplies actual structures/values.
