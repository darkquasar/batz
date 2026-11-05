# Contributing data sources and pipelines

Start with the [pipeline contract](docs/usage/pipeline-contract.md) and
[pinned macOS setup](docs/usage/ci.md). CloudTrail is an ordinary source example.
A routine contribution supplies:

1. **Source contract.** Describe accepted formats/events, output meaning, source
   time interpretation and limitations. Distinguish saved-file import from live
   acquisition. Logical event identity is optional; document reliable key scope
   and missing-key fallback if supplied. Missing identity must not block valid input.
2. **Manifest and acquisition configuration.** Declare routes, framing, adapter,
   staging/evidence/required-check models and outputs, with
   `normalization.project: repo://.`. See the [CloudTrail manifest](pipelines/aws-cloudtrail/pipeline.yaml)
   and [input configuration](pipelines/aws-cloudtrail/input/local-records.yaml).
   Discovery is automatic; no registry needs editing.
3. **Small source fixture and independent expectations.** Use invented or safely
   anonymized input with a checksum and provenance. The first declared explain
   example is the representative native contract fixture; its expectations execute.
   Other examples remain available for explicit preview. Choose expected values
   from the source contract. See the [CloudTrail fixture](pipelines/aws-cloudtrail/fixtures/wiring.json),
   [expectations](pipelines/aws-cloudtrail/fixtures/wiring.expected.json) and
   [fixture notes](pipelines/aws-cloudtrail/fixtures/README.md).
4. **Native models and tests.** Put SQL in `models/<pipeline>/` and native SQLMesh
   YAML tests in `tests/<pipeline>/`. Cover distinguishing mappings, malformed and
   unsupported records, source time and any identity policy using explicit fixture
   types. See [CloudTrail models](models/aws-cloudtrail/) and
   [native tests](tests/aws-cloudtrail/test_mapping.yaml). Reusable SQL belongs in
   `models/shared/`; source models cannot consume another pipeline's private input.

Run these exact commands, substituting your pipeline's manifest name:

```sh
ci/test fast
ci/test models aws-cloudtrail --mode native
ci/test models aws-cloudtrail --mode adapters
```

Native mode must pass independently of adapter-discovered schemas. Adapter mode
runs actual framing/fidelity and malformed-input checks, then feeds the actual
parsed schema and fixture through declared native roots, required checks, profile,
origin/accounting validation and declared expectations. Both modes also run as
`ci/test models aws-cloudtrail`. See the [passing criteria](docs/usage/pipeline-e2e.md).
No Go application E2E, per-source build tag or core-language test is required for
routine SQL content. New adapter/engine code adds its relevant component checks.

```text
+------------------+  selects  +--------------------------+
| Pipeline content | ========> | Own native + adapter     |
+------------------+           | contracts + fast checks  |
                               +--------------------------+

+------------------+  selects  +--------------------------+
| Core behavior    | ========> | Shared synthetic app     |
+------------------+           | and focused safety tests |
                               +--------------------------+
```

Shared tests own processing/publication, replay/recovery, ownership/cancellation,
strictness and preview. Do not copy their lifecycle matrices. Private pipeline SQL
selects its native consumers and adapter/contracts, without the shared app journey.
Native YAML test-only edits select native tests, including synthetic test edits.
Shared SQL selects native consumers; it also selects `ci/test app` when the shared
synthetic journey consumes it. The current repository fixture is self-contained;
a new real shared/global dependency also needs its consumed content included in
the fixture-copy helper. Core/runtime/collector code, runtime fixture content
and relevant dependencies/global configuration select shared app coverage. Unknown
impact broadens visibly or fails; deletion/removed edges retain former consumers.

Local and CI use the same commands and refuse empty, omitted or skipped selected
work. `ci/test app` is shared scope; `ci/test all-local` qualifies the full local
catalog when justified. The per-pipeline `ci/test e2e` lane is retired and fails.
Specifically justified pipeline qualification tests belong with that pipeline in
its owning repository. Local-file success never qualifies a remote API or local/R2
normalized parity; those explicit transport/platform obligations remain separate.

Keep content declarative: project Python, Jinja, hooks and symlinks are unsupported.
Use a fresh workspace for the current alpha layout. In the PR, name the source
scope, representative route/fixture, distinguishing expectations, commands/results
and unqualified transports. No new fixture framework or coverage ledger is needed.
