# Canonical symbolic Apply implementation map

Status: implementation decision. This is the minimum production cut from the
current relation forest to reusable, zero-copy function transformers. It adds
no second summary language and no second solver.

The precision prerequisite is recorded in
`formal_carrier_precision_review.md`: a plain registered product over formal
roots is not a complete function summary. It cannot retain `result = param`, a
guarded correlation, or an input-derived heap/path write. The callable artifact
therefore remains the existing `relationCode` plus its existing `ValueTerm`,
`PathTerm`, `Guard`, and `EffectTerm` arenas. Registered factors remain the
canonical abstract domains, but they are evaluated at the final binding rather
than used to erase the symbolic relation early.

## One representation, four distinct jobs

These jobs must not be collapsed into another all-purpose carrier:

1. `relationCode` is the reusable guarded relation. It owns control,
   `ValueTerm`, `PathTerm`, `Guard`, ordered `EffectTerm`, and correlated
   outcomes once per lexical function.
2. A `callFrameTerm` is the dense lexical binding recipe for one Apply. It maps
   the callee's Param/Capture/Global/Ambient roots to existing caller terms.
3. The registered product contains semantic residues and final abstract values.
   Values, keyspace, path evidence, dynamic index, heap identity, membership,
   user lattices, and the must-set lanes keep their existing registered
   Join/Widen/Narrow laws. There is no axis-specific interprocedural solver.
4. The one formal WTO owns recursive fixed-point cells. Cells are keyed only by
   `(relationVar, relationCode location)` and never by caller, concrete State,
   depth, route, or entry value.

This separation makes axis count irrelevant to call-graph multiplicity. Adding
an axis changes the registered product and the effects which read/write it; it
does not add a new Apply protocol.

## Existing machinery: retain, narrow, or delete

| Existing item | Decision | Canonical role after the cut |
| --- | --- | --- |
| `relationCode` and `boundaryStepApply` | Retain | Sole callable IR and sole interprocedural instruction. |
| `relationApplyRef{variable, frame}` | Retain | Exact link from one lexical Apply to target relation and binding recipe. |
| `callFrameTerm` | Retain, narrow | Dense target-root binding recipe. It must not identify an invocation or own solver cells. |
| `valueFrameResult(frame, slot)` | Retain, generalize evaluation only | Sole lazy caller-visible reference to a callee result. Resolution follows the frame into the target arena; no concrete-State callback and no caller-arena copy. |
| `valueCellResult` | Freeze-time only | Local scalar-equation syntax eliminated exactly once by `relationTermClosure`. It must not survive sealed `relationCode` and must not become a second call mechanism. |
| `relationTermClosure` | Retain | The one freeze-time closure/admissibility fence. It removes environment and cell vocabulary while retaining formal roots and `valueFrameResult`. Its current bounded local-cell import can remain in the first slice because it scales with lexical cells, not callers. |
| `TermRootBindings` | Generalize internally | Keep dense width/root validation. Replace its Apply use as a copying recipe with a borrowed/lazy binding view over the frame. |
| `RebaseTermDAGs` | Remove from Apply | It may remain temporarily for freeze-time local closure and isolated construction utilities. It may never import a sealed callee DAG at an Apply. Delete it when those construction callers are gone. |
| `RebaseEffectDAGs` | Remove from Apply | Target-owned effects are specialized directly under frame bindings and committed in source order. Remove the utility after its remaining construction/test users migrate. |
| `relationApplicationGuardPlan` | Retain, shrink | Store reachable target guards, target mu scope, and exact root-binding provenance. Remove caller-owned copied `boundAtoms` for ordinary Apply. |
| `relationInvocationPlan`, invocation namespaces/routes, `relationForestRuntime`, `emitApply` route construction | Delete atomically | They instantiate callee equation state per call route and are the non-compositional path. The formal region replaces their scheduling role. |
| `SpecializationContext.FrameResult` and boundary-State result projection | Delete after the cut | `valueFrameResult` is resolved from the stabilized target relation under the frame binding, not from a route-owned post-call State. |
| `formalRelationRegionInventory` and its `WTOPlan` | Retain and make executable | One caller-independent cell/equation universe for the complete lexical relation forest. |

No compatibility switch or legacy fallback is part of this map. The old route
runtime is removed in the same change that makes the formal region executable.

## Arena-qualified lazy binding without a new IR

Term numbers are arena-local. The implementation must therefore carry arena
ownership while traversing a call, but it does not need a new public term
language. The ownership is already expressible by existing identities:

- target value/path/guard/effect syntax is identified by `relationVar` plus its
  existing local term ID;
- the caller-to-target substitution is the existing caller-owned
  `callFrameTerm`;
- a target result observed by the caller is already the existing
  `valueFrameResult(frame, slot)`;
- recursive values are referenced by the existing formal cell identity.

The lazy evaluator carries a private stack entry `(owner relationVar, frame,
parent binding)` while walking existing target terms. This is an evaluation
view, not stored syntax and not a hash-consed node. Looking up a target root:

1. locate its dense offset in the target `Shape`;
2. read the corresponding `frame.values` or `frame.paths` term in the parent
   arena;
3. evaluate that term under the parent binding;
4. memoize the result for the duration of this Apply transaction.

The concrete specialization boundary may continue to use `BindingCursor`:
evaluate the dense frame argument terms into scratch value/path slices, borrow
them through a child cursor, evaluate target-owned terms, and discard the
scratch slices after the transaction. The cursor contains `product.Value` and
`pathdom.Path`; the sealed target arenas never change.

Symbolic composition does not manufacture a `product.Value`. It preserves the
existing arena-qualified target term and frame-binding chain until a final
root Apply needs concrete analysis output. Consequently:

- no callee `ValueTerm`, `PathTerm`, `Guard`, or `EffectTerm` is interned into a
  caller arena;
- no callee body `State` is constructed;
- no callee DAG node count changes after sealing;
- nested calls add binding-stack depth during evaluation, not syntax depth in
  either arena.

Memoization is transaction-local and keyed by the arena-qualified existing term
plus binding identity. It is not a semantic cache and is never published.

## Zero-copy Apply transaction

For an already-stabilized target relation, `boundaryStepApply` performs one
canonical transaction:

1. Validate `relationApplyRef` and its `callFrameTerm` against the sealed target
   `Shape`. This is the dense validation currently provided by
   `NewTermRootBindings`; do not clone its slices.
2. Borrow the caller binding view and construct the target binding view.
3. Select feasible target outcomes by evaluating the target-owned `Guard` DAG
   under that view. The Boolean DAG and mu scope remain target-owned.
4. Resolve the selected result `ValueTerm` and `PathTerm` roots lazily. A caller
   continuation reads them only through `valueFrameResult`.
5. Resolve each selected target-owned `EffectTerm` under the same binding view
   and apply it, in relation order, through the one canonical effect/state
   transaction. Object literals, path stores, invalidations, dynamic index
   mutations, allocations, keyspace, and user factors all use their existing
   registered semantics.
6. Publish the complete correlated result/effect transaction to the caller
   cell once. A partial or failed resolution publishes nothing.

There is deliberately no `RebaseTermDAGs`, `RebaseEffectDAGs`, body solve,
route construction, or result-State projection in these steps.

### Guards

`relationApplicationGuardPlan` currently says the right thing in its type
comment but still calls `rebaseRelationActionGuards`, importing target atoms
into the caller. Ordinary Apply must instead record only:

- the target `Guard`;
- its target `loopMuTerm` scope;
- each target atom's provenance and whether its leaf is a formal root or a
  target-local stabilized producer;
- the frame supplying formal-root leaves.

Guard evaluation then reads target atoms through the same lazy binding view.
The existing ROBDD/guard algebra stays canonical and exact. Definition BindIn
may need its existing sparse owner equality transaction, but it must not become
the ordinary Apply path.

### Effects

An `EffectTerm` remains owned by the target `EffectArena`. Evaluation resolves
its embedded existing value/path dependencies under the target binding view.
The resolved operation is handed directly to the canonical effect executor.
The source term is never recreated in the caller arena. Program order and
guard/outcome correlation are preserved before committing any state.

## Recursive tuple-mu and WTO ownership

The executable fixed point is the existing `formalRelationRegionInventory`:
one cell for each relation node, step, and outcome; one influence graph; one
`solve.WTOPlan`. Its cell payload is a guarded relation fragment over the
existing sealed arenas:

- reachability/guard decision;
- arena-qualified existing result/register terms;
- ordered arena-qualified existing effects;
- correlated outcome identity;
- registered residual factors only where the relation genuinely computes a
  factor rather than a symbolic dependency.

This is a private solver payload/view, not another serialized IR. It must not
contain `state.State`, a copied term node, an invocation route, or caller
identity.

At a WTO head, Join/Widen/Narrow operate componentwise:

- identical arena-qualified references are reused;
- guarded alternatives retain correlation with the existing Guard algebra;
- registered residual factors call their registered laws;
- recursive feedback points to the same formal cell and is never unrolled into
  a deeper term DAG;
- narrowing occurs only after the WTO component's ascending phase stabilizes.

An Apply influence reads the target's stabilized outcome cell and installs the
lexical frame binding. It does not create target cells or run a target WTO.
Self recursion and mutual recursion therefore converge once in the lexical
call-SCC, symbolically. Final concrete Apply specializes the stabilized
relation; it never restarts the recursive fixed point per caller.

Do not activate recursion on a partially symbolic payload. The recursive red
tests below are a hard implementation fence: inability to preserve a relation
without materializing State means the carrier is incomplete, not permission
to restore the route runtime.

## Exact 1/10/100-caller law

For the same sealed helper called by 1, 10, and 100 lexical call sites, all of
the following target quantities must be byte-for-byte/count-for-count equal:

- `relationCode` and arena node counts for values, paths, guards, effects, and
  control nodes;
- formal target cells, equations/influences, WTO components, and WTO heads;
- target cell evaluations required to stabilize the symbolic relation;
- recursive SCC iterations and target outcome payloads;
- retained target memory after sealing.

Only these quantities may scale with caller count:

- caller-owned `callFrameTerm`/`relationApplyRef` count;
- dense argument binding reads;
- transaction-local specialization work and final caller publications.

`ApplyInstantiations == caller count` is not enough: target evaluations must
remain identical. The existing
`TestRelationProgramFunctionalSummaryCallerScaling` already asserts the core
cells/equations/evaluations law and should be extended with arena, WTO, effect,
guard, route, and retained-memory counters. The final route count must be zero,
not merely constant.

## First implementation slice

The first slice is intentionally narrow but production-complete for acyclic
direct calls:

1. Freeze a target output index from existing `relationCode.outcomes` and
   reachable guarded outcomes. Do not invent a summary struct containing
   concrete values.
2. Add the private lazy binding/evaluation view over existing arenas and
   `callFrameTerm`. Reuse `BindingCursor` at the final concrete boundary.
3. Resolve `valueFrameResult` through that view.
4. Evaluate target-owned guards and ordered effects through the same view.
5. Make the formal region execute acyclic `formalRelationInfluenceCalleeOutcome`
   edges without constructing invocation routes.
6. Switch the sole production Apply to this path and remove the acyclic route
   runtime in the same commit.
7. Only after the acyclic slice is green, implement WTO tuple-mu payload joins
   and remove the remaining recursive route runtime atomically.

The slice is not allowed to fall back to `State` or DAG rebasing. Unsupported
symbolic syntax is a red implementation gap.

## Red tests before production wiring

1. **Identity at distinct bindings.** A sealed `id(p) = p` target is applied to
   two unequal caller values. Results remain unequal as required, target arena
   counts do not grow, and no target equation is evaluated twice.
2. **Guarded correlation.** A target returning `p` when truthy and a different
   value when falsy preserves the result/guard correlation at two callers.
   Ordinary Apply imports zero guard/atom nodes into caller arenas.
3. **Path and heap write.** A target writes an input-derived value to an
   input-derived path and returns it. Alias, keyspace, heap identity, dynamic
   index, and path evidence match the current full oracle with zero term/effect
   import.
4. **Effect order and atomicity.** Two guarded effects with observable order
   commit in source order; a failed dependency resolves no result and commits
   no prefix.
5. **Caller scaling.** Extend
   `TestRelationProgramFunctionalSummaryCallerScaling` to 1/10/100 with the
   exact structural/evaluation law above and an assertion of zero invocation
   routes.
6. **Self-recursive tuple-mu.** One recursive helper has finite lexical cells,
   one WTO head, stable exact results, no depth contexts, no term growth, and
   identical target stabilization counts for 1/10/100 external callers.
7. **Mutual recursion.** Two helpers share one lexical call-SCC fixed point;
   changing external caller count changes no SCC cell or evaluation count.
8. **Full semantics.** The complete oracle—not a curated subset—must be green
   with the route runtime deleted. Real Kickside timing and allocation/RSS
   profiles are measured after every completed slice under the repository RSS
   fuse and `GOMEMLIMIT=3GiB`.

## Deletion fence

The implementation is complete only when production searches show:

- no Apply call to `RebaseTermDAGs`, `rebaseDirectCallTermDAGs`,
  `RebaseEffectDAGs`, or `rebaseDirectCallEffectDAGs`;
- no per-invocation `relationForestRuntime`, invocation namespace, route, or
  child coordinate creation;
- no `SpecializationContext.FrameResult` backed by a post-call `state.State`;
- no ordinary-Apply `boundAtoms` copy;
- no sealed `valueCellResult`;
- exactly one formal WTO equation universe and one production Apply path.

Temporary dead code is not a completion state. Once the full oracle and real
application measurement are green, delete the superseded types and tests rather
than retaining a compatibility surface.
