# Inspect a local pipeline

Use the project-local CLI from the Batz checkout:

```sh
runtime/.local/batz explain pipelines/aws-cloudtrail.yaml --level 1
runtime/.local/batz explain pipelines/aws-cloudtrail.yaml --level 2
runtime/.local/batz explain pipelines/aws-cloudtrail.yaml --level 3 --example api-read --strict
runtime/.local/batz explain pipelines/aws-cloudtrail.yaml --level 3 --format json > /tmp/explain.json
```

Level 1 shows declarations and named outputs. Level 2 adds the local tracer's
source, decoding, parsed publication, staging and normalized boundaries plus native
asset identities. Level 3 runs the installed native Bento/SQLMesh/DuckDB stack on
an explicitly associated local example in private scratch, showing actual values
and recursive native structures. A registered example carries a SHA-256 and
reviewed semantic expectations. An explicit file needs an explicit --route:

```sh
runtime/.local/batz explain pipelines/aws-cloudtrail.yaml --level 3 \
  --route local_raw --example /absolute/path/native-cloudtrail.json
```

```text
+---------------+  native preview  +-------------------+  inspect  +----------------+
| Local example | ===============> | Private scratch   | =======> | Values + types |
+---------------+                  +-------------------+          +----------------+
```

Fields identify their evidence basis. A VARCHAR payload is labeled JSON text;
STRUCT/LIST fields stay nested. Timestamp strings retain native precision; source
time granularity remains separate. Large integers render as decimal strings.
Runtime-generated IDs, provenance paths and acquisition times are referenced in
samples and retained under runtime, rather than disguised as source values.
Compare all sections except runtime for deterministic semantic results.

The first native sandbox adapter is macOS ARM64. It denies network and credential
service access, unrelated file reads and writes outside private scratch. Actual
native qualification runs before projection. Other targets report unavailable;
Linux/container qualification remains later work. One file, 16 MiB, 10,000 records,
120/300-second collection/model deadlines, one SQLMesh worker, one DuckDB thread,
and a sampled 1 GiB scratch stop threshold apply. Brief scratch overshoot is
possible. RAM has no Batz cutoff; average/peak RSS and sample counts are reported.
At most 100 records per boundary are displayed; total row counts are separate.
Observed and unobserved preview branches are derived from the selected example
and native results. These descriptions live in the stable sample reasons; an
explicit fractional-time example is not given the registered fixture's coverage
labels. Unexecuted routes retain declared or unavailable evidence.

Scratch is retained at runtime.scratch for inspection. It is separate from any
production workspace; no production run or revision is created. The sample covers
one S3 GetObject/IAMUser event. Other variants and branch coverage are not inferred
from its success. Broader source support remains later work.

Default explain returns 0 with unavailable optional capabilities disclosed;
--strict returns 3 when requested capability cannot be established. Invalid input
returns 2, actual execution failure 1, and integrity/expectation failure 4. It never
fetches dependencies or fixtures. Project Python and symlinked native content are
refused. Use --format json for the versioned alpha report.

Preview admits only native SQLMesh SQL (including `@` macros) and YAML in .sql,
.yaml and .yml files, plus CSV seed data. Jinja is refused before scratch is
created or the native runtime starts: the delimiters `{{`, `{%` and `{#`, and the
`JINJA_QUERY_BEGIN`, `JINJA_STATEMENT_BEGIN` and `JINJA_END` markers in any letter
case. The check is a conservative text scan, so these markers are refused even
inside SQL strings or comments. CSV rows are data and are not scanned. The
diagnostic names the file, never its content. The scan is an admission rule, not
a sandbox; the scratch filesystem and network isolation above remain the boundary.

The qualified model configuration is JSON-compatible YAML in `config.yaml`,
with one `local` gateway and bounded DuckDB data/state connections. Both must
explicitly disable automatic extension installation and loading, with no declared
extensions. Unsupported connection settings report preview unavailable before
scratch is created. The scratch watchdog measures the entire writable preview
root, including paths outside the current processing stage.

Workspace-relative native configs, model projects and examples use the explicitly
selected workspace, including when it is outside the checkout. Without that
context they remain symbolic and unavailable. A runtime checksum mismatch returns
integrity failure (4) in both default and strict explain modes.
