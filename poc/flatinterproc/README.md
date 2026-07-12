# Flattened interprocedural WTO proof

This directory is an isolated executable proof for replacing the nested
"summary equation calls a complete body solver" shape with one equation system
over exact `(SummaryKey, CFG point)` flow cells and `(SummaryKey, boundary)`
summary cells. It does not change production analysis behavior.

## What the proof preserves

- Flow cells use `state.Domain(reg)`, so the carrier is the existing complete
  product state and is independent of the number or identity of registered
  lanes.
- Summary cells use `summary.NormalizedDomain(reg)`.
- Body operations reuse ordinary state reads, writes, joins, widening, and
  narrowing. The small node vocabulary is test IR, not a replacement transfer
  language.
- Exact known internal calls read their exact summary cell. A summary at Bottom
  produces no normal successor yet; it can grow monotonically when the callee
  returns.
- Explicit external calls preserve the current conservative Top-like return.
  Production unresolved/dynamic fallbacks must remain outside the known-key
  Bottom rule.
- Context cancellation returns the last published snapshot. A partial scratch
  generation is never published.

## Entry generations

The flat solve distinguishes two things that must not be conflated:

1. The entry state for a known exact context is an immutable input during one
   summary generation.
2. Every call still contributes an entry state to the *next* generation.

Known entries are therefore seeded once through `Initial`; call transfers do
not re-emit the same entry into the current WTO. Re-emission adds no information
and invents a caller/callee SCC. Calls do accumulate next-generation
contributions, including for already-known keys. If a summary growing from
Bottom makes another call reachable, the outer discovery transaction joins the
new contribution, rebuilds the equation shape, and solves the next immutable
generation.

This maps to the current production phases:

- `program.collectCallContextKeys` and its prepasses construct functions,
  contexts, and entry states before `query.Run`.
- `query.Run` receives fixed `query.Function` closures. It grows summaries but
  does not mutate the `programKeys` context entries during that run.
- materialization can call `refreshExistingCallContextEntriesFromResult` after
  the summary snapshot. A production flattened driver must treat such refreshes
  as a new generation, not mutate entry cells underneath an active WTO.

`TestFlatEntryGenerationJoinsProgressivelyReachableCallers` checks this seam: a
second, different contribution to one context is unreachable until the first
callee summary grows. Nested and flat results agree, and the final entry is the
join of both callers.

## Dependency graph

The declared influences are:

- flow point to its CFG successors;
- return point to its summary boundary;
- callee summary boundary to every call point that may read the same callee
  function. The POC deliberately over-approximates across exact context keys;
  production can sharpen this with the already-resolved call-context key, but
  correctness does not depend on that optimization.

Known immutable entry states are roots, not caller emissions. Recursive calls
still form real SCCs through `callee summary -> recursive call -> return -> same
summary`. Bourdoncle WTO schedules those SCCs, while ordinary acyclic call
chains follow dependency order instead of incidental `SummaryKey` order.

Dynamic context discovery changes the cell set, so the POC rebuilds the WTO
transactionally between generations. Production can use the same rule first;
incremental WTO insertion is unnecessary for correctness and should only be
considered if generation rebuild cost is measured as material.

## Monotonicity conditions for production

The production conversion is sound only if all of these remain true:

- A known exact internal key missing its current summary is Bottom/no normal
  return, matching the empty outcome currently returned by the exact-key paths
  in `internal/callresult/provider.go`.
- The unresolved external/dynamic path continues through callable refinement
  and `unresolvedFunctionCallOutcome`; it must not be silently changed to known
  internal Bottom.
- Every body-to-summary projection lane is a monotone equation over flow cells.
  Returns alone are modeled here. Obligations, aliases, heap/effect deltas,
  typestate, presence relations, suspension, and captures still need direct
  differential coverage before production wiring.
- Context identity is stable within a generation. A changed entry digest creates
  or joins the next generation; it never changes the meaning of a live cell.
- Widening stays at CFG loop heads and real recursive component heads. Narrowing
  recomputes flow first and then reprojects summaries from narrowed flow. The
  summary domain has no independent `Narrow`, so stale pre-narrow summaries
  cannot be retained by fiat.
- Heap/keyspace ownership and summary normalization stay at the same boundary.
  Flattening scheduling does not authorize sharing caller-owned mutable maps.

The largest remaining proof gap is `internal/projectsummary.FromResultContext`:
production currently projects a completed `body.Result` across many semantic
lanes. Those projections must become boundary equations, or a transactionally
recomputed projection over converged flow, without changing their polarity or
ownership rules.

## Differential scope

The POC differentially compares a local nested reference solver with the flat
solver for:

- direct call chains;
- recursion with a base return;
- multiple exact value contexts;
- unresolved external fallback;
- loop widening and narrowing;
- progressively discovered entry contributions;
- cancellation publication.

Diagnostics in this package are deliberately small (`no-return` and
`unknown-return`). They prove scheduling behavior, not parity with the full
production diagnostic oracle. Full fixtures, oracle, soundness, and byte output
remain mandatory gates for any production slice.

## Transfer model

For 24 wrapper functions plus the root, the flat graph has 74 equation cells.

| Dependency order | Nested transfers | Flat transfers | Result |
| --- | ---: | ---: | --- |
| Callees before callers | 148 | 74 | Flat visits each cell once; nested pays one confirmation solve |
| Callers before callees | 1,850 | 74 | 25x reduction; nested carries growth one body/layer/outer round |

The adverse order test requires more than a 4x reduction. The favorable order
test separately asserts the flat run performs exactly one transfer per declared
cell. This demonstrates the structural multiplier and its order sensitivity;
it does not claim the whole Kickside corpus will improve by 25x.

The representative automation census is recorded in
`AUTOMATION_CENSUS.md`. Its optimistic exact-context ceiling is only 1.61x for
summary point transfers, so this mechanism is **not** recommended for production
integration as the main performance architecture. The proof remains useful as
the scheduling model inside a broader compositional design, where exact context
body solves are removed rather than merely flattened.
