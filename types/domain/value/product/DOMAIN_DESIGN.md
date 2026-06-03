# AbstractValue Abstract Domain — Design Document

Status: DRAFT for review.
Scope: wire the existing `AbstractValue` operations into the `lattice.Lattice[AbstractValue]` contract established by design step A+B and used by `Condition` (rev 2). Per the per-domain rule: ONE domain at a time, top-to-bottom, no fixture invocation.

Distinct from `Condition`: this domain's algebraic primitives are **already correct** — Forge seq 288 (the commutative/associative/idempotent semilattice law fix) and the per-axis reduced-product machinery have been the authoritative implementation for prior sessions. The work here is the contract wiring plus law-harness verification — not algorithmic.

---

## 1. Concrete semantics being abstracted

`AbstractValue` is the reduced product of multiple per-axis abstract values that together describe a flow value at a CFG point:
- type axis (`typ.Type` carrier; recursive families interned)
- shape / structured-carry axis (per-field overlays)
- nilability / presence axis
- numeric range axis
- effect-rows axis
- escape axis
- ownership axis
- evidence axis
- identity-recursion axis

The concretization `γ(AbstractValue)` is the set of concrete runtime values admitted by the conjunction of per-axis constraints. Reduction propagates information between axes when one axis refines another (e.g. a presence narrowing to NonNil refines the type axis to drop the optional).

For this domain, soundness has already been argued by the per-axis tests (`types/domain/value/axis/*`), the convergence-order property tests (seq 288 — `convergence_order_test.go`), and the recursive-family law tests (`product/recursive_family_law_test.go`). This design re-states the contract using the universal `Lattice[T]` interface; it does not re-derive soundness from scratch.

---

## 2. Carrier

`product.AbstractValue` — existing struct in `types/domain/value/product/value.go:42`. Interned by a unique-table; equality is pointer/identity on the canonical node. Recursive families are folded by `RecursiveFamilyInterner`.

Special elements:
- `Bottom = product.Bottom()` — empty concretization.
- `Top = product.Top()` — full concretization.

---

## 3. Galois connection

```
γ : AbstractValue → 2^State
γ(⊥)        = ∅
γ(v)        = ⋂_axes γ_axis(v.axis)
γ(⊤)        = State
```

The per-axis γ functions are defined in `types/domain/value/axis/*/`. Reduction is the explicit `ReducedProduct` operation that pulls inter-axis refinements until stable.

Soundness contract: every transfer producing or consuming `AbstractValue` must be a sound over-approximation under γ. The existing tests pin this per axis; this design does not re-derive.

---

## 4. Partial order, equality

```
a ⊑ b   iff   γ(a) ⊆ γ(b)
a = b   iff   γ(a) = γ(b)
```

Implementation (rev 2 per Codex):
- `Equal(a, b) := product.Equal(a, b)` — pointer fast path then structural `nodeEqual` fallback (see `value.go:262`); interning probes the same `nodeEqual`. Not pure identity; representational mismatches still fall through to structural equality.
- `LessOrEq(a, b) := Equal(Join(a, b), b)` — the carrier/join-induced order: `a ⊑ b iff a ⊔ b = b`. This is antisymmetric with `Equal` by construction (if `a ⊑ b ∧ b ⊑ a` then `Equal(Join(a,b), b)` and `Equal(Join(a,b), a)`, so `Equal(a,b)`). Sound; incomplete w.r.t. γ-subset (some semantically-related values may not satisfy this carrier-level order).

There is a SECOND, distinct order: `Covers(a, b)` is the semantic coverage preorder — "every state b admits is also admitted by a". `Covers` is NOT the lattice order because `Covers(a,b) ∧ Covers(b,a)` does not imply `Equal(a,b)` (carrier-distinct alias vs bare; `unknown` vs `any`; see Codex review). A product-level test verifies `LessOrEq(a,b) ⇒ Covers(b,a)` (the lattice order is sound w.r.t. coverage; the converse may fail).

---

## 5. Lattice operations — direct wiring

```
Bottom() := product.Bottom()
Top()    := product.Top()
Join(a, b) := product.Join(a, b)
Widen(prev, next) := product.Widen(prev, next)
Meet — NIL (see §6)
```

All operations are existing package functions in `types/domain/value/product/value.go`. No new code, no adapter. The Domain variable wires them to the Lattice contract:

```go
var Domain = lattice.Lattice[AbstractValue]{
    Bottom:   Bottom,
    Top:      Top,
    Equal:    Equal,
    LessOrEq: func(a, b AbstractValue) bool { return Equal(Join(a, b), b) }, // carrier/join-induced order; antisymmetric with Equal by construction
    Join:     Join,
    Meet:     nil,   // see §6
    Widen:    Widen,
}
```

A separate non-LawSuite test asserts `LessOrEq(a,b) ⇒ Covers(b,a)` over the §10.1 sample. The lattice order is sound w.r.t. semantic coverage; the converse may fail because `Covers` admits carrier-distinct mutual covering (alias/bare, `unknown`/`any`).

---

## 6. Meet — intentionally absent

Per Lattice contract (§9.1 of `types/constraint/DOMAIN_DESIGN.md` and the lattice.go header): a domain whose forward-analysis usage does not require a meet may leave `Meet` nil; LawSuite skips the meet-side laws in that case.

`AbstractValue` is a forward-analysis domain. The reduced product has natural Join (per-axis least upper bound) but no natural Meet across all axes — Meet would attempt the greatest lower bound, which for some axes (recursive type interner, structured overlays) is not algebraically clean and is not used by any analyzer surface. Forcing a Meet to satisfy harness completeness would invent semantics with no consumer.

The Lattice contract permits this. The harness must support it.

**Harness change required** (per Codex finding #3):
- `domainValid` at `laws.go:77` currently rejects `Meet == nil` as malformed. Relax it: a `Lattice` value with non-nil `Bottom`, `Top`, `Equal`, `LessOrEq`, `Join`, `Widen` is valid; `Meet` is optional.
- Every meet-side law (`checkMeetIdempotent`, `checkMeetCommutative`, `checkMeetAssociative`, `checkMeetLowerBound`, `checkAbsorption`) is gated on `s.Domain.Meet != nil`. When `Meet` is nil, those laws are skipped with no diagnostic. Absorption requires BOTH ⊔ and ⊓, so it is also gated.
- The `lattice.go` doc-comment for `Lattice[T].Meet` is updated to explicitly say "may be nil for forward-only domains; LawSuite skips meet-side laws when nil".

This is forward-compatible: a future domain that provides a `Meet` gets the full suite automatically.

---

## 7. Widen — existing, verified

`product.Widen(prev, next AbstractValue) AbstractValue` is the existing per-axis widening, reduced-product across axes. It uses `convergence.MergeForConvergence` underneath for the type axis. Soundness (over-approximation) and termination (ACC) are pinned by:
- `types/domain/value/convergence_order_test.go` — commutativity / associativity / idempotency / order-independence under shuffled folds (seq 288).
- `types/domain/value/convergence_test.go` — general convergence behavior.
- `types/domain/value/product/recursive_family_law_test.go` — recursive-family canonicalization, ensuring widening collapses to a stable representative.

The Lattice law harness re-checks these laws against the Domain wiring. Any future regression in `product.Widen`'s algebraic properties surfaces as a LawSuite failure.

---

## 8. Public API surface (corrected per "no adapter" directive)

Existing — unchanged:
```
type AbstractValue struct { ... }
func Top() AbstractValue
func Bottom() AbstractValue
func Join(a, b AbstractValue) AbstractValue
func Widen(prev, next AbstractValue) AbstractValue
func Equal(a, b AbstractValue) bool
func Covers(a, b AbstractValue) bool
```

NEW — one variable:
```
var Domain = lattice.Lattice[AbstractValue]{ ... }   // §5
```

No adapter type. The `Domain` is one variable in `types/domain/value/product/value.go` (or a new `lattice.go` alongside) wiring existing functions to the contract.

---

## 9. Harness change (additive)

`types/lattice/laws.go`: gate every meet-side law on `s.Domain.Meet != nil`. Total change ~15 LOC across five `check*` functions. The harness's own self-tests still pass because the presence lattice in `laws_test.go` provides a Meet, and the test cases for broken lattices either preserve or violate Meet explicitly.

Self-test addition: one new sub-test `TestLawSuite_HandlesMissingMeet` in `laws_test.go` constructs a join-semilattice (Lattice value with `Meet: nil`) and asserts `Run` completes without invoking the meet laws.

---

## 10. Test plan

Package-local to `types/domain/value/product/` and `types/lattice/`. No fixture-suite invocation.

### 10.1 LawSuite conformance — `lattice_test.go` (NEW in product/)

Sample (rev 2 per Codex — single-point structural coverage was not enough; must include the carrier-distinct mutual-cover cases that would falsify a `Covers`-based `LessOrEq`):
- `Bottom`, `Top`
- `FromType(typ.Boolean)`, `FromType(typ.Number)`, `FromType(typ.String)`, `FromType(typ.Integer)`
- `FromType(typ.Nil)`, `FromType(typ.NewOptional(typ.String))`
- `FromType(typ.Unknown)`, `FromType(typ.Any)` — these mutually-cover but are not Equal; the join-induced order is the only one that gets antisymmetry right.
- a bare record + the same record wrapped in `typ.NewAlias(...)` — carrier-distinct alias coverage case (alias_roundtrip_test.go:63 family).
- two distinct aliases over the same target — same family.
- a numeric range / exact-value AbstractValue constructed via `New` rather than `FromType`.
- a non-Top effect-rows AbstractValue.
- an `ownership.Unique` AbstractValue.
- an `escape.Fresh` AbstractValue.
- two distinct recursive families (record self-references with different field sets).
- a union AbstractValue (two-member union via `FromType`).

LawSuite runs every join-side law, partial-order law, and widening law. Meet-side laws skipped (Meet is nil). Absorption skipped (requires both ⊔ and ⊓).

In addition (separate test, not LawSuite): for every sample pair (a, b), assert `LessOrEq(a, b) ⇒ Covers(b, a)`. This pins that the carrier-induced lattice order is sound w.r.t. semantic coverage.

Acceptance: every law passes; every (LessOrEq ⇒ Covers) check passes.

### 10.2 Harness change verification — `laws_test.go` (extended in lattice/)

`TestLawSuite_HandlesMissingMeet` constructs a no-meet Lattice value (presence lattice with `Meet: nil`) and asserts `Run` completes with zero meet-law invocations. A mockReporter captures absence.

### 10.3 No new domain-specific tests

The per-axis tests, convergence tests, and recursive-family tests pre-date this design and pin the algebra. This design does not duplicate them.

---

## 11. Documentation

- This file (`DOMAIN_DESIGN.md`) in `types/domain/value/product/`.
- Doc-comment on `Domain` variable referencing this document.
- Doc-comment on `lattice.LawSuite.Run` updated to state that Meet-side laws are gated on `Meet != nil`.

---

## 12. Out of scope

- Anything Condition-domain related (already closed).
- The other finite-height domains (NumericRange, PathPresence, LengthBound) — they follow only after this one is closed.
- design step D Kildall refactor.
- Per-axis algebra (already correct per seq 288).

---

## 13. Acceptance criteria

- `LawSuite[AbstractValue].Run` passes on the §10.1 sample.
- `TestLawSuite_HandlesMissingMeet` passes in `laws_test.go`.
- `go test ./types/domain/value/... ./types/lattice/...` is green.
- `go test ./compiler/check/... ./types/...` regresses no tests vs HEAD (only the pre-existing `tests/errors` 180s hang remains).
- No fixture-suite invocation.
- No adapter type introduced.
- The `Domain` variable is one declaration in `types/domain/value/product/`; no per-domain wrapper struct.

---

## 14. Implementation plan

ONE child agent, scoped strictly to `types/domain/value/product/` and `types/lattice/`. The agent receives:
- this design document
- the lattice header and laws.go for the harness shape
- the Condition-domain landing (commit `93d2170b`) as the precedent

Agent steps:
1. Update `types/lattice/laws.go`: relax `domainValid` (allow nil `Meet`); gate `checkMeetIdempotent`, `checkMeetCommutative`, `checkMeetAssociative`, `checkMeetLowerBound`, and `checkAbsorption` on `s.Domain.Meet != nil`. Add `TestLawSuite_HandlesMissingMeet` in `laws_test.go`.
2. Update `types/lattice/lattice.go` doc comment for `Lattice[T].Meet` to state it may be nil for forward-only domains.
3. Add `Domain` variable in a new file `types/domain/value/product/domain.go` (per Condition's precedent at `condition_lattice.go`). Use the join-induced `LessOrEq` from rev 2: `Equal(Join(a, b), b)`.
4. Add `domain_test.go` in `types/domain/value/product/` applying `LawSuite[AbstractValue]` to the §10.1 sample (carrier-distinct mutual-cover cases included). Add the separate `LessOrEq ⇒ Covers` semantic-soundness test over the same sample.
5. Verify the scoped tests pass; verify `./compiler/check/... ./types/...` does not regress (one final run, no fixtures).
6. Report per-criterion against §13.

The agent does not invoke fixtures. The agent does not modify any other directory.

On the prior agent's token-exhaustion experience: this work is bounded (~50 LOC code + ~150 LOC tests + harness gating). If token-limited mid-work, the harness gating step lands first because it unblocks every subsequent join-semilattice domain.
