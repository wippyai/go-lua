# Exact lexical function transformer POC

This isolated POC tests the smallest exact bridge between the current concrete
CFG solver and reusable lexical transformers. Production packages and Kickside
are unchanged.

## Proven slice

`Compile` accepts one nonrecursive (acyclic) lexical CFG containing only
`PathAssignment` node operations and `BranchPathRelation` edge operations. It:

- rejects every other `factflow.FactsInput` family by reflection, so a newly
  added family fails closed rather than silently disappearing;
- rejects cycles, facts outside the CFG, relations on non-branches, unused
  expression paths, contextual value sources, and incomplete root bindings;
- compiles the CFG once into topologically ordered point rows, with node effects
  before ordered edge guards;
- packs lexical-to-caller root bindings and structurally rebases assignments,
  source paths, and both branch-relation operands;
- executes a bound transformer in one topological pass, joining predecessor
  contributions before interpreting a join row, without a worklist/fixpoint;
- records the input state at every point and exit, matching `transfer.Result`.

The execution path invokes `ApplyConcretePathAssignment` and
`ApplyConcreteBranchPathRelation`, the same factapply kernels as production.
The differential runs 1,024 combinations (64 distinct caller root bindings ×
16 parameter-value bindings) through both the compiled transformer and the
ordinary concrete solver, comparing the complete 17-lane State at every CFG
point and exit. The diamond fixture has equality/inequality guards on opposite
edges, branch-local assignments, a join, and a post-join assignment, so a path
replay that skipped join correlation would fail.

This is an exact *row-instantiation* proof, not yet a closed-form symbolic
summary. It still replays each supported semantic kernel once per invocation.
That limitation is intentional and measured rather than hidden.

## Result

Ryzen 9 7950X3D, Go 1.23.3, three repetitions:

| Operation | Time | Allocations |
| --- | ---: | ---: |
| ordinary concrete body solve | 58.0–58.5 us | 46,024 B / 472 |
| bound topological row instantiation | 30.3–30.9 us | 12,896 B / 106 |
| caller root binding | 2.94–3.00 us | 8,000 B / 64 |

The exact topology/phase collapse is about **1.9×** and removes 77.5% of
allocations on this fixture. That is useful but decisively below the required
4×+ architectural win. The remaining cost is the replay of concrete semantic
kernels and State construction. A production-worthy next stage must evaluate a
precomputed guarded boundary effect representation (with compact/root-bound
storage) instead of replaying the body. This POC establishes the exact oracle
and fail-closed boundary for that work.

Run:

```sh
go test -race ./poc/functiontransformer
go test -run '^$' -bench BenchmarkLexicalFunction -benchmem -count=3 ./poc/functiontransformer
```
