# S1 native macOS budget decision

Status: owner disposition recorded below; the original proposal is historical.

The bounded [native probe](../contributing/probes/macos_memory.py) ran on macOS
15.6.1 ARM64 with the pinned Python. All three attempts to set a 64 MiB limit
(RLIMIT_AS, RLIMIT_DATA, RLIMIT_RSS) were rejected before allocation. See the
[complete result](../contributing/evidence/s1/macos-memory-probe.json). This does
not prove every possible macOS enforcement mechanism is unavailable. It does
mean these primitives do not qualify the accepted process-tree limits.

The current runner enforces input bytes/records, stage timeouts, one workspace
owner and one collector/model at a time. DuckDB has its own memory setting, which
is not a total Python/SQLMesh/child-process memory cap. Process-tree memory and
aggregate scratch/output are not yet enforced. S1 is not accepted.

Proposed owner choice for the S1 native macOS fixture only: retain 1 GiB collector
and 2 GiB model thresholds and 1 GiB private scratch/output threshold, but define
them as sampled watchdog thresholds. A native supervisor would sample process
RSS and attempt file sizes every 100 ms, kill the process group on excess or
measurement failure, settle its children, and return operational failure before
publication. Qualification must exercise the actual child tree, timeout,
cancellation and disk-growth cases before acceptance. RSS excludes some kernel
and shared allocation costs; allocations between samples can overshoot, and
escaped/reparented descendants need explicit containment/settlement evidence.
This proposal therefore does not promise hard instantaneous ceilings or an
untrusted-code sandbox. It does not change S2 offline isolation requirements.

Alternative: retain the hard maxima and keep S1 unaccepted until a native
mechanism satisfying them is demonstrated. A Linux VM/container cannot establish
native macOS qualification; substituting one would need a separate support decision.

The decision is necessary because it changes the accepted resource invariant,
not because ordinary implementation requires another permission check.

## Owner disposition

The owner answered "Yes, check frequently and stop on excess", then superseded
the RAM part with: "for now, you are allowed to use as much ram as needed, just
monitor it so we have an idea of averages". RAM is monitoring only, with no Batz
cutoff; scratch retains the sampled stop threshold. The accepted contract now
documents nullable RAM limits and per-stage measurement fields. The original
proposal above is retained as the decision history, not the current RAM policy.
