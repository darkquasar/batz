# Framework-owned event fixture

These invented records exist only to test shared Batz behavior. They are not a
vendor integration. Shared app roots copy this fixture, its native models/tests,
root configuration and shared runtime code; no production pipeline is loaded.

`id`, `subject` and `observed_at` supply small output examples. `observed_at` is
already normalized UTC text cast to TIMESTAMP_NS, not a vendor time parser.
An optional `disposition` selects `rejected` or `unsupported`; otherwise a record
is mapped. The production collector receipt rejects non-object decoded payloads.
Logical identity is absent. Shared time probes test preservation, and retained
`original_time` values exercise payload fidelity without interpreting offsets.

The `other` binding in `TestAppSharedRecoveryJourney` uses the two original JSONL
records for action/actor/native time/payload/origin assertions, reusing an existing
successful run. Do not add a standalone synthetic E2E or copy vendor lifecycle tests.
