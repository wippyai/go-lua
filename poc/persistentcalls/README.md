# Persistent canonical call cells POC

This package is an isolated alternative to the feature-by-feature symbolic
transformer experiment. It keeps the production intraprocedural transfer
functions and `state.State` entry domain unchanged. Each lexical function owns
one workspace whose entry is the monotone join of observed caller entries. The
existing production WTO solver schedules the mutually dependent equations:
caller summaries influence callers, while caller-produced entries influence
callees. All lanes in `state.Domain(reg)` participate automatically; there is no
17-lane capability switch in this mechanism.

The summary is separately generic over a caller-supplied lattice. The tests use
both `state.State` and a production-shaped mask containing returns, obligations,
facts, effects, and context metadata. A real adapter must supply the lattice for
the existing complete program summary; `state.State` alone is not claimed to be
the production summary.

## Ownership and invalidation

A workspace has one canonical owner: its lexical function cell. Entry growth
with an unchanged exact dependency-revision vector calls `ExtendEntry`. If any
callee summary revision changes, the workspace is discarded and rebuilt from
the cell's complete canonical entry. Each solve records every summary read and
revalidates its revision before accepting the candidate. WTO convergence is
private; all summaries publish in one transaction, and cancellation/error drops
partially mutated workspaces and publishes nothing.

This is the distinction the old hybrid resume path missed. It retained CFG
cells produced under callee revision N and continued their ascending chain after
the callee refined to N+1. Those cells were not an approximation produced by the
new transfer function, so resuming could stabilize at stale imprecision. The
regression test deliberately gives a caller workspace that caches its first
callee result: unsafe reuse remains Bottom after refinement, while canonical
revision invalidation rebuilds it and observes the refined result.

## Precision and comparison

One lexical cell joins caller contexts. This is sound, but it is not generally
byte-identical: a test demonstrates a context-sensitive body whose separate
string/number entries remain precise while their merged entry becomes Top.
Production migration therefore needs bounded context cells keyed by the
existing semantic context identity, or a proof that a body is distributive over
the merged entries. Context overflow can merge monotonically, but must be a
visible precision policy rather than an accidental cache effect.

Compared with explicit symbolic transformers:

- Persistent cells cover every current and future State lane and reuse existing
  transfer semantics. Their main new machinery is canonical ownership,
  dependency revisions, context partitioning, and transactional scheduling.
- Symbolic transformers can instantiate calls much more cheaply and retain
  parametric correlation, but every root/effect/State lane needs a sound
  summarize/substitute implementation. The automation census showed the
  currently modeled symbolic syntax is still below one percent of transfers.
- Persistent cells are therefore the lower-complexity broad-coverage candidate.
  They do not promise solve-once cost: a callee refinement correctly rebuilds
  affected callers. The benchmark question is whether monotone entry extension,
  WTO locality, and bounded contexts remove enough repeated whole-body work.

Run:

```sh
go test -race ./poc/persistentcalls
go vet ./poc/persistentcalls
```
