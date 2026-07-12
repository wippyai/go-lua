# Automation topology census verdict

The flattened WTO POC proves equivalence for its modeled lanes and a 25x
structural win on an intentionally adverse dependency order. That is not the
production expectation: the production summary query already schedules summary
dependencies, and most exact cells are solved only once.

This census reuses the behavior-neutral `BodySolveAttribution` output from the
same cold 112-unit `kickside.automation` run recorded by the regional-call
census. No flattened solve path or new production instrumentation was enabled.

For each exact `(unit, SummaryKey, phase=summary)` cell, the optimistic
one-generation cost is:

```text
ceil(PointTransfers / BodySolves)
```

This gives the flattened design every benefit: one existing intraprocedural
convergence per exact context, no extra global recursive-SCC revisits, no context
generation rebuilds, and no boundary-projection overhead.

| Metric | Count |
| --- | ---: |
| Units | 112 |
| Lexical/base cells | 2,398 |
| Semantic context cells | 1,802 |
| Exact cells | 4,200 |
| Current summary body solves | 5,564 |
| Current summary point transfers | 200,577 |
| Optimistic one-generation transfers | 124,780 |
| Transfers removed at the theoretical flat boundary | 75,797 (37.8%) |
| Maximum summary-transfer speedup | 1.61x |

The solve-count distribution explains the ceiling:

| Solves per exact cell | Cells |
| ---: | ---: |
| 1 | 3,029 |
| 2 | 995 |
| 3 | 161 |
| 4 | 13 |
| 5 | 2 |

The 1,171 repeated cells account for 137,102 current transfers and an optimistic
61,305 one-generation transfers. The other 3,029 cells already pay exactly one
summary body convergence and contribute 63,475 transfers that flattening cannot
remove.

As a second optimistic bound, grouping every exact context by lexical
`(unit, FuncRef)` and charging only the largest observed one-generation cost per
function leaves 67,710 transfers across 2,398 functions. That is 2.96x versus
the current summary phase before charging symbolic evaluation, instantiation,
global SCC revisits, entry generations, prepass, materialization, or diagnostics.
It is a bound for a true compositional/symbolic design, not for exact-context
flattening.

## Decision

Do **not** integrate the exact-context global WTO as the main performance
architecture. Its measured optimistic ceiling is 1.61x for the summary transfer
phase and less for end-to-end lint, below the required plausible 4x corpus win.
The 25x synthetic case remains a valid worst-case property, but it is not the
automation topology.

The census points back to the larger compositional boundary: eliminate exact
context body solves by solving each lexical function against symbolic boundary
values and instantiate its transformer at calls. Even that boundary is only an
optimistic ~3x reduction of summary transfers on this slice, so reaching the
end-to-end target also requires collapsing duplicate prepass/materialization
work and reducing the cost/allocation rate of each full-state transfer. A new
architecture should be judged against all three phases, not summary solves
alone.
