# Automation cone census verdict

A temporary, behavior-neutral adapter printed the existing
`BodySolveAttribution` counters for two categories: ordinary summary solves and
dependency-change solves handled by the retained regional path. The adapter and
cache lived under `/tmp`; no census code remains in production.

For each exact body/context cell, the counterfactual full cost of its dependency
updates was estimated from that cell's observed ordinary point transfers per
solve. On the 112-unit `kickside.automation` slice:

| Metric | Count |
|---|---:|
| Summary body solves | 5,564 |
| Summary point transfers | 200,577 |
| Dependency-change solves | 1,364 |
| Actual dependency-change transfers | 75,187 |
| Estimated clean/full dependency-change transfers | 77,429 |
| Transfers saved by regional replacement | 2,242 |

The exact regional mechanism saved about 2.9% of dependency-change transfer
work and 1.1% of total summary transfer work. Of 1,171 affected exact cells:

- 1,038 cost exactly the same as their estimated full solve;
- only 130 saved any transfer work;
- none cost less than 25% of a full solve;
- one cost less than 50% of a full solve.

The mechanism is sound and remains valuable hardening, but the real dependency
cones are overwhelmingly whole-body (or conservatively fall back to it). This
boundary cannot provide the order-of-magnitude performance improvement. Making
the cone selector less conservative without changing semantic dependencies
would recreate the precision hole; the next performance design must reduce the
underlying dependency graph/work per transfer rather than cache or resume it.
