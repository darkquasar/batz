# Accepted S2 alpha contract

Owner accepted the proposal on 2026-10-29 by "ok go ahead with next slice" and
reaffirmed "continue, do the 2nd slice, always consider uber go style guide".
The proposal remains the accepted design input; this packet gives exact schemas.

ExplainReport keeps batz/v0alpha1 with an explicit alpha amendment: level is 1–3;
optional stages, structures, samples, resolution and runtime sections are added.
S1 documents remain readable. New producers emit cumulative sections; missing
observations are unavailable, never invented. Structures recursively retain native
type text; JSON text is labeled separately from STRUCT/LIST. Samples use lossless
string encoding for native timestamps and large integers. Runtime contains variable
identities, source-location substitutions and measured resources, outside stable
comparison. The schema in schemas/v0alpha1/explain-report.schema.json is normative.

Level 2 is declarative graph inspection. Level 3 uses only qualified native local
preview, or explicit unavailable diagnostics (default 0, strict 3). Invalid selection
is 2; operational execution failure 1; semantic/integrity failure 4. The RAM owner
amendment remains in force: sampled RSS averages/peaks, no Batz RAM cutoff.

Workspace/Case/PipelineBinding document envelopes have closed versioned schemas.
The four shared sections are typed maps; native adapter validation remains required
before execution. Parameter defaults follow the declared parameter type. Public
context flags are --workspace, --binding, --case, --binding-id and repeatable --input.
Case entries use the same binding shape without envelope fields. A relative CLI
path is caller-relative; YAML path values retain their declaring-file origin.
