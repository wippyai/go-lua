# Automation census verdict

The behavior-neutral production bridge from commit `1ac62bfd0` mapped this POC
onto the current engine and counted hypothetical canonical workspaces without
enabling reuse. It was removed after measurement so the disproven mechanism has
zero production overhead.

Two cold `kickside.automation` runs against the detached engine commit produced
the same aggregate:

| Metric | Count |
|---|---:|
| Units | 112 |
| Lexical functions | 2,398 |
| Semantic context cells | 1,802 |
| Summary body solves | 5,564 |
| Required workspace builds | 5,564 |
| Callee-revision resets | 1,364 |
| Eligible monotone entry extensions | 0 |
| Entry-shrink resets | 0 |
| Routing resets | 0 |
| Theoretical full body solves avoided | 0 |

The live Kickside tree regenerated some content-addressed unit IDs between the
runs, but every aggregate above remained identical. The structural conclusion
does not depend on those IDs.

The result disproves lexical/context persistent workspaces as the broad speedup
under the sound ownership rule. Every repeated solve in this slice follows a
callee-summary refinement. The canonical rule must rebuild in that case,
because CFG cells computed under revision N are not an ascending chain for the
transfer function at revision N+1. Relaxing that rule recreates the old hybrid
precision hole; enforcing it avoids no body solves.

The next viable reuse boundary is lower: exact regional recomputation inside a
body. A changed callee contribution must replace the old point contribution,
then transactionally recompute the downstream CFG/WTO closure from preserved
unaffected boundaries. Unlike resume, replacement supports non-monotone local
changes such as missing/unknown becoming known.

## Production mapping retained from Stage 0

- Summary payload: the complete `summary.NormalizedDomain`, not just returns or
  `state.State`.
- Context cell: exact `summary.SummaryKey{Ref, Entry}`; every `EntryKey` field is
  part of the semantic partition.
- Entry domain: `body.Config.EntryState` under the configured production State
  domain, including every registered lane.
- Workspace candidate: `body.RetainedPreparedSession`, owned by one exact cell.
- Dependencies: every read recorded by `trackingSummaryReader` and
  `pointSummaryDependencyTracker`, including absent reads.
- Scheduler/publication: query/solve owns interprocedural convergence; summaries
  publish only after convergence and exact dependency revalidation.
- Failure: cancellation or validation failure aborts pending updates, releases
  unpublished sessions/results, and publishes nothing.
