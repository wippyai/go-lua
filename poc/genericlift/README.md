# Generic transformer lifting feasibility proof

This package is an isolated registration and transaction proof. It deliberately
does **not** claim to lift the current production transfers: the required
semantic operation stream does not exist yet.

## Verdict

Catalog-derived, fail-closed transformer capability is straightforward. Generic
lifting of the existing prepared transfer closures is not.

The production seams are concrete:

- `state.laneSpec` owns Bottom, Top, Equal, LessOrEq, Join, Widen, and Narrow
  over fields of concrete `state.State`. Its typed get/set closures are erased
  internally; `LaneCatalog` exports lane IDs and concrete State domains.
- `transfer.NodeTransfer` and `transfer.EdgeTransfer` are whole-state
  `state.State -> state.State` functions.
- `factapply` reads concrete product/path/heap facts, branches on them, and
  writes several State lanes in one operation. There is no symbolic carrier or
  interceptable operation algebra.
- State lanes are not summary lanes. The 17 State lanes map into 19 normal
  return fact lanes plus other Summary fields. Projection also consumes
  assignment/call metadata. For example, path invalidation is derived jointly
  from assignments, dynamic-index facts, and effect deltas.
- keyspace paths, captures, allocations, and heap identities require explicit
  caller rebasing. An exit-State snapshot cannot recover parameter dependence,
  branch correlation, or allocation freshness.

Consequently, adding `Summarize(StateLane)` and `Substitute(StateLane)` to the
State catalog would be an attractive but incorrect abstraction. It would either
lose relational behavior or reimplement most transfer semantics inside lane
adapters.

## What this POC proves

`DefaultRegistry` derives its ordered roles from
`state.DefaultLaneCatalog().LaneSet()`. Missing adapters are installed as
unsupported roles; they do not disappear from coverage. The smallest common
adapter is:

```go
type LaneLifter interface {
    Lane() state.LaneID
    Build(BuildContext) (LaneProgram, Support)
}
```

Each `LaneProgram` keeps its typed symbolic payload private and instantiates
through a transaction-owned builder. The registry never passes an `any` payload
from one adapter to another. A missing lane, unknown operation, malformed
program, or late instantiation failure rejects the complete transformer and
publishes no partial patch.

The tests prove:

- all current State lanes are present in catalog order;
- every unimplemented lane defaults to contextual fallback;
- one adapter enables one selected lane without an engine switch;
- a future catalog lane automatically fails closed;
- unknown operations fail closed;
- duplicate/orphan adapters are rejected;
- late failure commits no partial result.

`Operation` is intentionally only a prerequisite marker in this POC. Production
does not yet provide the operation stream needed to populate it.

## Smallest honest implementation path

The shared semantic seam should be below Lua syntax and above concrete State
methods:

```text
bound WIR + factflow facts
          |
          v
 immutable semantic operation plan (per prepared body)
          |                         |
          v                         v
 concrete operation interpreter    symbolic operation interpreter
          |                         |
 existing State/17 lanes            guarded lane programs + effects
          |                         |
          +------ differential -----+
```

This is an operation-plan refactor, not a new Lua analyzer:

1. Compile the already-lowered `factflow.Facts`, call sites, branch facts, and
   observation plan into immutable semantic operations. Do not add syntax cases.
2. Move existing `factapply` behavior behind the concrete handlers one operation
   family at a time. The concrete handler remains the oracle.
3. Let each lane adapter consume the same operation plan and build its private
   symbolic program. An unsupported operation/lane rejects the whole function.
4. Use the existing call-boundary fact catalogs and
   `engine/registry.BindOrdered` pattern for ordered, exhaustive projection and
   instantiation. Unlike the strict production boundary registries, composition
   fills missing handlers with contextual fallback so adding a lane remains
   safe before its adapter lands.
5. A successful symbolic solve must publish both summary facts and the compact
   observation/diagnostic projection. That is what permits prepass, summary, and
   materialization to collapse into one semantic solve.
6. Keep the current contextual engine as a per-function fallback. Mixed
   symbolic/concrete functions compose only at the existing normalized summary
   boundary.

The operation handlers must expose lane reads/writes and ownership/rebasing
requirements. Merely tracing concrete executions is insufficient: an unvisited
branch may be feasible for a future binding, and a concrete trace cannot prove
its absence.

## Comparison with existing POCs

| Design | Reuses complete transfers/lanes | Removes context body solves | Measured/structural ceiling | Verdict |
| --- | --- | --- | --- | --- |
| `flatinterproc` exact-context WTO | Yes | No | 1.61x summary transfers on automation | Correct proof, insufficient payoff |
| `symboliccall` expression/effect model | No; semantics are hand modeled | Yes for modeled slice | Optimistic lexical floor below | Reuse its guard/correlation/allocation laws, not its implementation seam |
| Generic lifting directly from State catalog | No operation algebra exists | Theoretically | Not implementable without broad genericization | Reject |
| Shared semantic operation plan | Concrete interpreter reuses/refactors `factapply` | Yes, fail-closed by operation/lane | Combined phase/context model: 5.92x optimistic | Smallest plausible main path |
| Relational context batching | Calls current concrete transfer per partition | No, unless partitions merge | At best exact-context/phase-collapse class | Useful fallback optimization, not the main 4x lever |

The existing `poc/symboliccall` capability registry has the right exhaustive
intent, but its `any` payloads are not connected to State extraction or prepared
transfers. Its useful assets are the semantic laws: distinct parameter/capture/
vararg roots, guarded correlated rows, contravariant requirements, allocation
rebasing, effect-row widening, and atomic fallback.

The earlier `analysis/check/fixpoint/symbolic` package was removed rather than
kept as a second production summary model. It hand-modeled values, guards,
requirements, heap deltas, and calls without consuming the engine's prepared
semantics, so extending it would have created another analyzer. The reusable
invariants remain requirements of this operation-plan design: boundary roots
stay namespace-distinct until binding; caller values and concrete heap state
never enter transformer identity; expression ownership cannot cross a plan;
and every complexity limit fails closed or widens observably instead of
silently truncating behavior. The isolated `poc/symboliccall` package retains
the corresponding algebraic proofs.

## Measured phase and context cost model

A behavior-neutral rerun of the same 112-unit `kickside.automation` slice
printed existing `BodySolveAttribution` for every phase. The only adapter change
was printing prepass and materialization rows in addition to summary rows; the
sorted diagnostic payload is byte-identical to the prior regional census.

| Phase | Exact cells | Body solves | Point transfers |
| --- | ---: | ---: | ---: |
| Prepass | 3,546 | 3,546 | 120,209 |
| Summary | 4,200 | 5,564 | 200,577 |
| Materialize | 2,280 | 2,467 | 80,244 |
| **Total** | — | **11,577** | **401,030** |

The exact-context flat census found a best-case one-generation summary cost of
124,780 transfers. Grouping exact contexts by lexical `(unit, FuncRef)` and
charging the largest observed one-generation cost per function gives an
optimistic symbolic cost of 67,710 transfers across 2,398 functions.

| Counterfactual | Transfer-equivalent cost | Maximum reduction |
| --- | ---: | ---: |
| Current three phases | 401,030 | 1.00x |
| Context collapse only; retain prepass/materialize | 268,163 | 1.50x |
| Phase collapse only; retain exact contexts | 124,780 | 3.21x |
| Context + phase collapse | 67,710 | 5.92x |

A 4x target permits at most about 100,257 transfer-equivalents. The combined
design therefore has an allowance of about 32,547 over the lexical floor—48% of
that floor—for symbolic branching, transformer construction, instantiation,
and projection before it falls below 4x. This makes the combined architecture
plausible, not proven. Either context collapse or phase collapse alone is
structurally insufficient.

The next gate is not more symbolic syntax. It is one representative
operation-plan slice that runs the existing concrete handler and a symbolic
handler differentially, includes every used lane through this fail-closed
registry, publishes summary plus observation projection, and measures total
build + instantiation cost against that 100,257-equivalent budget.
