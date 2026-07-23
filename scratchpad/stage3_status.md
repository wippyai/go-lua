# Stage 3 status

Implemented the shadow-only acyclic transaction VM in `analysis/check/fixpoint/equation`.

- Entry binding closes the sole formal entry term and rejects body/parameter mismatches.
- The VM validates a deterministic acyclic readiness schedule, resolves kernels by the exact `(KernelID, ContractID)` pair, and invokes only those bound kernels.
- A partial transaction, audit failure, kernel failure, conflicting output, missing dependency, or cycle returns no published closure.
- Output closures include values, outcomes, diagnostic candidates, and allocation rekeys.
- Shadow mode invokes production and the bound evaluator independently, then compares the complete canonical published closure.

Current differential corpus: 3 named acyclic witnesses — `identity`, `guarded-return`, and `copied-store`; pass rate: 3/3 (100%).

Validation completed: equation tests, `go build ./...`, and transformer/state/factapply tests.

Fixture census gate: blocked by the existing full-fixture run. The first
`go test -vet=off -run '^TestFixtures$' .` invocation ran for 600.316 seconds
and hit Go's test timeout while freezing a formal coordinate-dependency
closure; it emitted zero normalized `--- FAIL:` fixture names, so neither a
382-name comparison nor a second meaningful pass is available. Stage-3 files
are isolated to the equation package and are not called by that production
fixture path.
