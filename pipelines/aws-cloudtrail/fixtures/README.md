# Synthetic local CloudTrail fixture

api-read.json is adapted from the synthetic native Records example in the
pipeline specification. It is invented test data, contains no incident evidence
or credentials, and makes no claim to cover general CloudTrail behavior.
The unmapped_example field exercises preservation of an unknown source field.
The timestamp has second precision; future nano probes must be labelled synthetic
stack probes separately from this representative input.

`wiring.json` contains two deliberately synthetic nanosecond probes (UTC and
`+10:00`) of the same instant at two nanoseconds past the second. They exercise the native source-time
boundary in the representative adapter/native contract; they do not imply source clock accuracy.
The actor and preserved payload marker are independently chosen expected values.
`wiring.expected.json` executes for this first manifest example. Native source tests
retain distinct one-/two-nanosecond UTC/offset epochs and complete final lineage expectations.
`api-read` remains an explicit preview example with its own expectations.

The representative expectations assert the exact timestamp string
`2026-10-25T12:00:00.000000002` for both records. This compares the actual native
JSON serialization without a DataFrame dtype conversion and protects offset
interpretation. The source-owned native tests separately retain the 1 ns/2 ns
distinction and complete final time/origin projections.
