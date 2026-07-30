# go-lua checker: Lean mechanization pilot

Lean 4 proofs for the two smallest-ranked obligations in the mechanization
roadmap of
[`analysis/architecture/soundness_obligations.md`](../analysis/architecture/soundness_obligations.md#mechanization-roadmap):

1. **B4/F1 — depth-exhaustion dual polarity** (`invariants.md` Rule 1)
2. **A1 — per-lane lattice laws**, piloted on the `Placement` lane

No Mathlib dependency: both proofs go through with core Lean tactics
(`decide`, `omega`, structural induction) over finite/structural carriers, so
the build has zero external dependencies and stays fast.

## What is proved

### `GoLua/PlacementLattice.lean` — obligation A1, `Placement` lane

Mirrors `analysis/domain/placement/placement.go`: the `Value` type
(`Bottom < Stack < OwnedHeap < SharedHeap < Unknown`), `Join`/`Meet`/`Widen`/
`LessOrEq`/`Equal`, modeled as a Lean inductive with an explicit `rank`
embedding into `{0..4}`.

| Theorem | Mirrors |
| --- | --- |
| `allValues_complete`, `allValues_length` | finiteness of the carrier |
| `le_refl`, `le_trans`, `le_antisymm`, `le_total` | `TestPlacementLatticeLaws`: order axioms |
| `total_order_chain` | `TestPlacementOrderJoinMeetAndWiden`'s chain assertion |
| `join_idem`, `join_comm`, `join_assoc` | `TestPlacementLatticeLaws`: join laws |
| `join_bottom_identity_left/right`, `join_unknown_absorbs_left/right` | bottom-identity / top-absorption |
| `le_iff_join_eq` | order-consistent-with-join |
| `join_ge_left`, `join_ge_right`, `join_least_upper_bound` | join is a least upper bound |
| `meet_idem`, `meet_comm`, `meet_assoc` | `TestPlacementLatticeLaws`: meet laws |
| `absorption_join_meet`, `absorption_meet_join` | absorption |
| `widen_eq_join`, `widen_overapproximates_left/right` | `Widen := Join` and its over-approximation |
| `order_join_meet_widen_table` | `TestPlacementOrderJoinMeetAndWiden`'s concrete case table |
| `chain_stabilizes` | A1's widening-chain termination clause, specialized to height 4 |

`chain_stabilizes` models a fixpoint solver's ascending iteration as
`chain g s0 (n+1) = Widen (chain g s0 n) (g (chain g s0 n))`, where `g` is a
transfer function of the *current* abstract value (the shape a real solver
round actually has), and proves the chain is fixed forever from round 4
onward — the placement lattice's height. The supporting lemmas
(`chain_change_rank_lt`, `chain_fixed_from`, `join_change_rank_lt`) are the
finite-height argument spelled out as a real induction, not asserted.

### `GoLua/DepthPolarity.lean` — obligation B4/F1, `invariants.md` Rule 1

A minimal abstract model of a bounded recursive relation and its may-contain
dual, mapped in comments to the real Go functions: `stopDepthPair`
(`analysis/type/subtype/guard.go`, tested by
`TestSubtypeDepthExhaustionFailsClosed` and
`TestValueProofAdmissibleRuntimeCastDepthExhaustionFailsClosed`) and
`numericForMayContainNumber` (`analysis/check/readmodel/api.go`).

| Theorem | Statement |
| --- | --- |
| `boundedPositive_sound` | `boundedPositive d t = true → actualPositive t = true` |
| `boundedMay_sound` | `boundedMay d t = false → actualMay t = false` |
| `boundedPositive_exact_of_sufficient` | budget `≥ size t` reproduces the exact answer exactly |
| `boundedMay_exact_of_sufficient` | dual exactness |

`Term` is a small binary tree (`atom` / `node`) standing in for a recursive
type-graph node; `actualPositive`/`actualMay` are its exact, unbounded
conjunctive/disjunctive answers (well-founded on `Term`'s own finite
structure — the point being proved is about the *budget mechanism*, not
graph cyclicity). `boundedPositive`/`boundedMay` carry an explicit fuel
parameter that returns `false`/`true` respectively at exhaustion, mirroring
the two Go functions' polarity exactly. The soundness theorems are the
mechanized form of Rule 1: exhaustion can never make a positive relation lie
true, and can never make its may-contain dual lie false.

## How to build

```sh
cd proofs
lake build
```

Toolchain: `leanprover/lean4:v4.32.0` (pinned in `lean-toolchain`, installed
via `elan`). No `lakefile` dependencies to fetch. Build completed clean:

```
✔ [4/4] Ran GoLuaProofs/GoLua:default
Build completed successfully (4 jobs).
```

All theorems are sorry-free; `#print axioms` on every theorem in both files
reports only `propext` and `Quot.sound` (Lean's standard trusted-kernel
axioms) — no `sorryAx`, no `Classical.choice`.

## Next 3 candidates (from the roadmap)

3. **D3 — noninterference / exact factorization law.** Already a clean
   quantified law at two altitudes:
   `project_I(x) = project_I(y) => P(x) = P(y)`. Would take: pick one
   concrete boundary morphism as pilot `P`, define its denotational
   semantics over an abstract store/projection model, and prove the
   implication — a genuine spec-first proof, since no executable lawsuite
   for this exists in Go yet either.
4. **C2 — substitution exactness at BindIn/BindOut.** A capture-avoiding
   substitution lemma once `relationCode`'s guard/value vocabulary gets a
   typed calculus (atoms, binders, a substitution function). Would take:
   define the calculus, define substitution, prove the standard
   substitution lemma against the existing Go test suite
   (`relation_forest_definition_equations_test.go`) as the model to match.
5. **E4 — witness equality stricter than structural equality.** Would take:
   an inductive/coinductive type-graph model with a separate stable-identity
   carrier, defining witness equality as
   `RecursiveIdentitySet(a) = RecursiveIdentitySet(b) ∧ TypeEquals(a,b)`, then
   proving it strictly finer than `TypeEquals` alone and that canonical
   encoding is injective on witness-equal classes. Best done before the Go
   implementation plan lands (no stage of it exists in the tree yet), matching
   the register's own prose-first-then-mechanize order.
