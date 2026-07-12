# Root-write and value-guard boundary POC

This isolated POC expands the exact acyclic boundary slice beyond
`PathAssignment` and `BranchPathRelation`. It compiles a small function with
four ordinary root assignments, a two-way `ValueRefinement` guard, a join, and
six boundary-invisible local statements into one immutable topological plan.
Production code and Kickside are unchanged.

The plan deliberately reuses the newly shared semantic transactions:

- every root write runs through `ConcreteRootAssignmentPointExecutor.Apply`,
  preserving its immutable input/evolving output boundary and complete sidecar
  ordering;
- every branch constraint runs through `ApplyConcreteGuardRefinement`;
- caller execution requests either boundary-only Summary/exit state or a sparse
  set of point-input observations; and
- the differential compares every CFG point, exit State, and normalized Summary
  under the full domain and each of the 17 single-lane domains.

Admission is explicit and fail-closed. The fixture admits all 4 root writes and
both edge guard applications. Cycles, non-root writes, negated/falsy contextual
guards, and every other `FactsInput` family are rejected. In particular,
object-literal and call sidecars are rejected and are **not** claimed as covered:
this POC proves only the object/call-free transaction slice. Reflection ensures
a future fact family is rejected until its ordering is modeled.

Representative Ryzen 9 7950X3D results (five repetitions):

| path | time | bytes | allocations | speedup |
|---|---:|---:|---:|---:|
| ordinary concrete body solve | 55.7–57.3 us | 48,320 | 431 | 1.0x |
| boundary-only exit + Summary | 12.9–13.0 us | 13,144 | 51 | 4.3–4.4x |
| two sparse observations | ~13.1 us | 14,168 | 55 | ~4.3x |

This clears the 4x architecture bar for the admitted fixture, but it is not yet
a closed-form symbolic transformer: the four admitted semantic transactions
are still replayed once per invocation. The useful result is narrower and
actionable: topology/phase collapse plus boundary-only materialization remains
worth more than 4x even after adding exact root writes and correlated value
guards, while the next coverage step must model populated object/call sidecars
rather than assuming them away.

Run:

```sh
GOCACHE=/tmp/go-build-rootguard go test -race ./poc/rootguardeffects
GOCACHE=/tmp/go-build-rootguard go test -run '^$' -bench BenchmarkRootGuardBoundary -benchmem -count=5 ./poc/rootguardeffects
```
