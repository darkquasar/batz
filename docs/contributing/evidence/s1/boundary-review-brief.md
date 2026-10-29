# S1-A/B named boundary review brief

Fresh independent read-only native review, required by plan-execute-cx at the
S1-A/B boundary. This is not final whole-plan review R and cannot accept S1's
unqualified resource maxima. Scope: verified receipt and fresh evaluated exact
membership are prerequisites for publication; prior reader revisions and parsed
evidence survive model failure; project-supplied Python does not execute.
Include the narrow workspace-owner boundary. Review the actual current code,
contracts, native configs/SQL, and tests, including code the tests miss.

App root: /Users/martinpopolito/GitHub/batz. Specs root:
/Users/martinpopolito/GitHub/batz-specs. Both are unborn main; no base or HEAD
commit exists. All delivered source is untracked. Do not rely on git diff.
Exact source inputs: checked-source.json and boundary-review-inputs.json beside
this brief. Verify their hashes. Plan/spec hashes are in that package. Read the
plan S0/S1, accepted first-tracer-contract.md and preserved proposal; later slices
are not claimed implemented. User accepted the first-tracer proposal and local
checksum-verified setup with "ok cool I agree".

Observed checks: build-cli, all Go tests, vet and go mod verify exit 0, with exact
commands/environment in check-results.json and full readable logs. CLI run and
ordinary DuckDB query both exit 0 (cli-result.json); actual run/receipt/inventory
schemas pass (native-wire-validation.json). Intended failing tests and their
subsequent passing logs are retained; receipt adversarial tests exercise the
private verifier using actual native collector output, not the full CLI. Other
native cases use app.Execute with real collector/SQLMesh processes. The original
single public CLI behavior has RED/GREEN evidence, followed by narrow loops for
integrity, precision, timestamp oracle, receipt evidence, model selection,
route qualification, source shape and membership reporting.

Known limitations: process-tree memory and private scratch/output limits are
not yet enforced, and native rlimit probes failed. Owner policy decision remains
pending; do not waive or accept it. S2 offline isolation and full selection,
S3 restart/recovery/durable failure reporting, S4 reconciliation, S5 remote auth,
S6+ sources, S8 full isolation and S9 packaging/portability remain later tasks.
No Linux native/container qualification, R2 access, deployment, Git commit,
push or cleanup has occurred. Level 1 explain only. Project Python rejection
has a direct preflight test. Native published timestamps include exact synthetic
nano/offset/undated tests, separately from the second-granular original fixture.

Granted checks: read files, hashes, source and retained evidence; git status/log
metadata as appropriate (HEAD absent). No execution that writes source/cache,
workspaces or runtime state, no installs/network/cloud calls. The coordinator
has already run checks. State any missing evidence and return all findings with
severity, file:line, consequence and proposed correction; do not infer a pass
from green tests. Use the requesting-code-review-cx/code-reviewer.md rubric.
No writing, delegation, successors, permission bypasses or model/provider changes.
Return the complete report to the coordinator and stop, with no background jobs.
