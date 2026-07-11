# Transactional regional recomputation

Status: design constraint for a future implementation. This is not an
authorization to wire the existing `solve.Session` back into production.

## Why the current resume session is insufficient

The current solver stores only the joined value of each cell. If equation `p`
previously emitted `A` to `q`, and a later evaluation emits `B`, resume computes
`q = q join B`. It cannot remove `A`. This is correct only when every new
evaluation's contribution is provably above its previous contribution. That is
not a safe generic contract for changing call-summary environments, strong
updates, refinements, or changes in the set of outgoing emissions.

A minimal counterexample is a two-cell max lattice. The first transfer of `p`
emits `2` to `q`; after replacing the transfer it emits `1`. A clean solve gives
`q = 1`, while the current resume checkpoint remains `q = 2`. In an analysis
lattice the analogous result is stale imprecision. Removing an emission
entirely has the same defect.

There are three additional blockers:

* `Resume` mutates its retained ascending checkpoint directly. Cancellation or
  an uncovered dynamic dependency can therefore leave partial work retained.
* widening visit/change history is retained across the changed equation, so a
  resumed result need not have the canonical precision of a clean schedule;
* `transfer.TryRun` handles `Resume` before schedule selection, so a requested
  WTO solve resumes through FIFO and never executes its WTO plan.

The existing session can remain checkpoint infrastructure for callers with a
separately proven inflationary equation change. It must not implement general
regional recomputation.

## Equation ownership

For a cell set `C`, retain the equation system in this form:

```
value[d] = Abstract(d, Initial[d] join join(outputs[p][d] for p in C))
```

Each invocation of `Transfer(p)` owns one complete output bag `outputs[p]`.
Multiple emits from `p` to the same destination are joined inside that bag.
Re-evaluating `p` replaces the whole bag atomically, including destinations
which are absent from the new bag. Initial values are a separate immutable
boundary owner. `Abstract` applies after the complete destination aggregate,
not per contribution, because it need not distribute over join.

The retained checkpoint also owns, per equation:

* the exact set of cells read by its last committed evaluation;
* the exact set of destinations emitted by that evaluation;
* reverse readers and output edges derived from those owned sets;
* the pre-narrowing value, revision, and WTO component identity of every cell;
* widening visit/change history for the current checkpoint generation.

Read and output edge sets are replaceable, not append-only. Otherwise removed
dynamic dependencies leak memory and make invalidation grow forever.

## Invalidating a region

An update names equation owners whose dynamic environment changed. Examples
are CFG points that read a changed callee summary or points whose explicit
initial state changed.

Build the influence graph from both kinds of committed edge:

* `read d while evaluating p` gives `d -> p`;
* `p emits to d` gives `p -> d`.

Collapse that graph with the declared CFG graph into SCCs. The initial region
is the union of every SCC containing a changed owner and every SCC reachable
from it in the condensation DAG. Whole-SCC invalidation is mandatory: resetting
only some members retains cyclic contributions from the old generation.

Before replay, on transaction-owned scratch:

1. remove every output bag owned by an equation in the region;
2. reset every region cell to its immutable initial value joined with committed
   contributions from owners outside the region;
3. clear region read/output edge sets, visits, and widening-change counters;
4. run the region in its canonical WTO order, extending output bags by owner;
5. stabilize nested components and validate that no influence escaped the
   selected region;
6. clone the converged overlay for regional narrowing/public projection. The
   narrowed clone is never the retained checkpoint.

The region is a forward closure, so a changed region must not influence a cell
outside it. If a newly discovered edge does so, extend with the target's
downstream SCC closure only when the edge is forward in the existing WTO.
A backward or cross-component edge can merge SCCs and requires a clean full
solve with a rebuilt plan.

FIFO history is globally order-dependent when widening is present. Exact
regional equivalence is therefore offered only for the canonical WTO schedule.
FIFO callers fall back to a full FIFO solve. With WTO, stabilized predecessor
components are boundary inputs and replaying a downstream component from its
clean boundary reproduces the same component schedule as a clean WTO solve.

## Narrowing

The retained checkpoint is always pre-narrowing. Publication narrows a scratch
view and never writes narrowed values back to the checkpoint.

Regional narrowing seeds region cells from their initials plus committed
outside-to-region contributions, evaluates only region owners in canonical
order, and applies the same bounded narrowing rule to all declared and
emitted-only cells owned by the region. If narrowing observes or emits an edge
outside the validated closure, discard the transaction and full-solve.

Running global narrowing is a sound first implementation if it is cheaper to
prove, but it must still be scratch-only. It reduces the performance win and
does not remove the need for replaceable ascent contributions.

## External summary dependencies

The existing `trackingSummaryReader` records dependencies for a whole body.
Regional invalidation needs point ownership. While evaluating a CFG equation,
summary reads must be attributed to that active point as `(summary key,
normalized digest/presence)`. A changed summary key invalidates precisely the
points whose committed dependency token differs.

Reads performed outside an active equation (preparation, result projection, or
an uninstrumented provider) are deliberately unowned. A changed unowned read is
a full-body fallback until that phase has its own safe incremental projection.
An empty tracked set is not proof of independence when the nameable summary
universe changed; preserve the existing zero-dependency universe fence.

## Transaction and cancellation

`BeginUpdate` creates a copy-on-write overlay over one immutable committed
generation. All value changes, contributions, dynamic edges, versions,
widening history, and observations go to the overlay. The public checkpoint is
never mutated during replay.

Commit is one pointer/generation swap of the pre-narrowing ascent overlay after
all of these succeed:

* WTO ascent and scratch narrowing converge;
* cancellation is checked after the last transfer callback;
* every dynamic influence is covered by the validated region/plan;
* all per-point external dependency tokens are complete;
* optional differential validation passes.

Cancellation, panic-to-error conversion, an uncovered edge, or projection
failure discards the overlay and publishes nothing. Solve-local versions may
continue monotonically within the overlay, but are never serialized or compared
across committed generations.

## Minimal API seam

Keep this separate from `Session` until the laws are proven:

```go
type RetainedSystem[Cell comparable, State any] struct { /* private */ }

type Update[Cell comparable, State any] struct { /* private overlay */ }

func BuildRetainedWTO(
    ctx context.Context,
    sys EquationSystem[Cell, State],
    plan *WTOPlan[Cell],
) (*RetainedSystem[Cell, State], error)

func (r *RetainedSystem[Cell, State]) BeginUpdate(
    changed []Cell,
    transfer Transfer[Cell, State],
    versioned TransferVersioned[Cell, State],
) (*Update[Cell, State], error)

// Run converges the pre-narrowing ascent overlay.
func (u *Update[Cell, State]) Run(ctx context.Context) error
// Publish narrows a clone of the overlay; it does not commit it.
func (u *Update[Cell, State]) Publish(ctx context.Context) (
    values map[Cell]State,
    versions map[Cell]uint64,
    err error,
)
func (u *Update[Cell, State]) Commit() error
func (u *Update[Cell, State]) Abort()
func (r *RetainedSystem[Cell, State]) Release()
```

`BuildRetainedWTO` performs a clean solve and captures owned contributions and
dependencies. `Publish` is scratch-only. `Commit` is illegal before successful
`Run`, `Publish`, and caller-side projection validation. The body/program layer owns one retained system only for
the duration of one outer summary fixed point; it must never enter the
cross-unit summary cache or an exported `body.Result`.

## Exact fallback conditions

Discard regional scratch and perform a clean solve when any of these holds:

* cell list/order, CFG shape, lattice/selected lanes, initial-state function,
  abstraction, widening policy/delay, or schedule identity changed;
* the committed checkpoint was not produced by the same canonical WTO plan;
* a changed external dependency lacks an owning CFG point;
* a missing dependency becomes nameable under a changed summary universe and
  ownership cannot be proven;
* a new dynamic read/output edge is backward or merges WTO components;
* an emitted-only cell cannot be assigned to one equation owner;
* the invalidation closure exceeds a configurable fraction of cells/edges, so
  retaining and copying it costs no less than a clean solve;
* contribution/edge retention exceeds its memory budget;
* transfer monotonicity, persistent-state ownership, or deterministic replay is
  not guaranteed by the caller;
* cancellation, callback error, panic, or final projection error occurs;
* differential mode finds any normalized state, summary, diagnostic, manifest,
  or artifact mismatch.

The fallback is transactional as well: build a fresh retained generation and
swap it only after successful full convergence.

## Memory ownership and bounds

Contribution retention changes storage from roughly `O(V)` states to `O(E)`
states. Keep output bags compact (inline zero/one/small fanout), retain immutable
persistent state snapshots without deep cloning, and account edges plus
estimated retained state bytes. A per-body budget must trigger full solve and
drop retained detail rather than grow without bound.

Only one committed generation and one active overlay may exist. After commit,
release replaced output bags, dynamic edge sets, and old region values once no
publisher references them. `Release` must clear all maps/slices and be called
when the outer summary fixed point ends. Cross-unit caches retain normalized
summaries only, never a retained equation system.

## Required adversarial tests

Before production wiring, differential tests against a clean WTO solve must
cover:

1. an owner contribution decreases;
2. an owner stops emitting to a former destination;
3. an owner changes two destinations in one evaluation;
4. a changed owner is inside a nested SCC;
5. a new forward edge expands the region;
6. a new backward edge forces full fallback;
7. a dynamic read disappears and its reverse edge is removed;
8. widening history is reset for an invalidated component;
9. narrowing creates an emitted-only cell;
10. cancellation after one or more scratch emits preserves the old checkpoint;
11. an external summary read outside an active point forces full fallback;
12. repeated updates remain memory-bounded and deterministic.

The acceptance oracle is a clean solve under the same WTO schedule. Normalized
point states and all public projections must match; merely being mutually
ordered or producing the same current diagnostics is not sufficient for the
default path.
