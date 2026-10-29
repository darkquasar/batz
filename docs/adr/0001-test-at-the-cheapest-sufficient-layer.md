# ADR-001: Test at the cheapest layer that proves the behavior

Status: Accepted, 2026-10-29. Scope: Batz development and review.

## Context

During Slice 2, repeated Bento/SQLMesh/DuckDB startup dominated test time.
Ordinary preview fixtures already contained one record; reducing their size
would not remove that startup cost. Several validation cases and report
assertions were unnecessarily starting their own complete pipelines.

JSON validation can use invented data. Claims about actual native values,
structures, precision, lineage and isolation require real execution. These are
different proof obligations and should have different test costs.

## Decision

Choose the least expensive test layer that can establish the required behavior:

| Behavior | Test layer |
| --- | --- |
| Parsing, expectation validation, precedence, types and path resolution | Direct module calls with small invented inputs |
| CLI selection, diagnostics and report contracts | Public CLI interface in process; assert that native execution does not start where appropriate |
| Actual engine values, structures, precision and adapter integration | A small real native journey with compatible assertions grouped together |
| Publication, retries, ownership, cancellation and filesystem/network isolation | Real lifecycle or operating-system tests with the independent state the requirement needs |
| Scaling behavior | Explicit workload measurements for the relevant feature; separate from ordinary correctness cases |

```text
+---------------+  direct call  +-------------------+
| Invented JSON | ============> | Validation module |
+---------------+               +-------------------+

+---------------+  real execution  +--------------------+
| Small fixture | ===============> | Native integration |
+---------------+                  +--------------------+
```

1. Test the same module interface used by production. Keep validation callable
   without launching an engine. Retain end-to-end tests that prove the wiring
   and error classification across those interfaces.
2. Reuse a real result for compatible assertions within one journey. Check its
   values, structures, provenance and contract together. Do not create another
   pipeline merely to inspect another field of the same result.
3. Keep fresh runs when freshness is the assertion. Repeatability still needs
   two independent runs. Independent failure/publication cases retain isolated
   state. Avoid process-global mutable fixtures, test-order dependencies and
   shared mutable workspace caches.
4. Use the smallest fixture that distinguishes the behavior. Keep two distinct
   records when duplicate/omitted membership cannot be proved with one. Resource
   and concurrency tests retain the real activity needed to exercise their limits.
5. Avoid blanket Cartesian products of features, sources and platforms. Add
   combinations when an accepted requirement or concrete interaction risk needs
   them; preserve required adapter/platform qualification.
6. Use focused checks during development and the composed suite at completion.
   Follow the existing TDD and verification requirements. This ADR does not waive
   acceptance checks, native evidence, isolation or reviewed expectations.
7. Measure significant test-cost growth. Record workload, pipeline launches,
   fixture volume, timings and retained assertions. Do not infer app-wide
   complexity or future suite duration from a small timing sample.

## Evidence and consequences

The initial application of this decision produced these observations on the
same native macOS setup:

| Selected checks | Before | After |
| --- | --- | --- |
| Preview/report/expectation group | 7 fresh pipelines, 28.16 s | 3 fresh pipelines, 11.57 s |
| Expectation validation, included in that group | 3 fresh pipelines, 12.84 s | 8 cases in one Python process, 0.04 s |

Native structure/value assertions still execute the real stack. The two
repeatability runs remain independent. The full suite retained lifecycle and
failure-isolation coverage and passed with the additional review regressions.
These are individual observations, not performance guarantees or proof that the
application has no quadratic algorithms.

New validation cases should normally add inexpensive checks instead of engine
startups. Native test time should grow with justified independent journeys,
rather than with every assertion or an automatic cross-product of dimensions.
Reviewers should check the chosen layer, fixture size, repeated setup and state
isolation when tests are added.

See [development and completion commands](../contributing/local-setup.md),
[the before measurement](../contributing/evidence/s2/completion/test-cost-before.json),
[the after measurement](../contributing/evidence/s2/completion/test-cost-after.json),
and [composed verification](../contributing/evidence/s2/completion/final-check-results.json).
