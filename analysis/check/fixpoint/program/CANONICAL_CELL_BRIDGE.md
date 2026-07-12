# Canonical persistent-cell production bridge

Stage 0 is observation only. It does not call `ExtendEntry`, retain a new body
workspace, resume CFG cells, or change summary scheduling.

The isolated `poc/persistentcalls` concepts map to production as follows:

| POC concept | Current production owner | Required production rule |
|---|---|---|
| Summary lattice | `summary.NormalizedDomain(reg)` over the complete `summary.Summary` | Returns, obligations, aliases, normal-return facts, heap objects, typestate, effects, suspension, and future summary lanes remain in the existing normalized lattice. |
| Function/context cell | `summary.SummaryKey{Ref, Entry}` | `Ref` is lexical identity. All fields of `EntryKey` are the semantic context partition; Stage 0 enumerates the struct fields rather than hardcoding Values/Facts/References. |
| Abstract entry | `body.Config.EntryState` under `state.DomainWithOptionalLanes` | Nil lane selection means every lane in `state.DefaultLanes`. Entry growth may extend only under the same context, routing, and dependency revision vector. |
| Workspace | `body.RetainedPreparedSession` / `RetainedPreparedUpdate` | A session is owned by one exact function/context cell. Existing retained regional updates are not treated as canonical entry extension by Stage 0. |
| Dependency vector | `trackingSummaryReader`, `pointSummaryDependencyTracker`, normalized summary equality, and immutable summary-universe identities | Every summary read, including an absent read, participates. Any changed callee payload/revision resets the workspace before solving. |
| Interprocedural scheduler | `query.Driver` equations over `SummaryKey`; production `solve` supplies revisions and WTO plans | A production bridge must make the query driver the sole owner of interprocedural SCC/WTO state. A body/session must not preserve cells across a callee revision independently. |
| Transaction | query result map → `summary.NewSnapshotOwnedNormalized`; retained `attempt`/`pending` commit | Candidates remain private until the complete SCC converges and dependency revisions revalidate. No partially converged summary is published. |
| Cancellation/release | program context checks, retained attempt aborts, `RetainedPreparedSession.Release`, `Result.ReleaseTransient`, run-level deferred `retained.Release` | Canceled/failed transactions publish nothing and release unpublished sessions/results. |

The old hybrid resume path violated the dependency rule: CFG cells created under
callee revision N were resumed after revision N+1 as though they were one
ascending chain. They were not outputs of the new transfer function, so stale
imprecision could survive. Canonical ownership permits monotone entry extension
only while the complete dependency vector is unchanged. A callee revision,
routing change, entry shrink/incomparability, or context change rebuilds from
the cell's full canonical entry.

## Stage-0 counters

`Stats.CanonicalCellCensus()` reports exact function/context cells in stable key
order, the current State lane catalog, every `EntryKey` dimension, observed body
solves, theoretical initial/reset builds, monotone/equal-entry reuse candidates,
resets classified by callee revision, entry shrink, context, or routing, and the
number of full body solves a canonical cell could theoretically avoid.

The avoided count is an upper bound. It proves available repeated work, not that
incremental propagation is free. Production wiring is justified only if frozen
corpus measurements show a large enough multiplier and byte-identical context
partitions.
