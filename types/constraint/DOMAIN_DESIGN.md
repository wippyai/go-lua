# Condition Abstract Domain — Design Document (rev 2)

Status: REVISED per Codex adversarial review of rev 1 (`.codex-out-condition-design.txt`).
Scope: complete redo of the Condition path-condition abstract domain by the book of Cousot–Cousot abstract interpretation. This is the FIRST and ONLY domain being redone in this pass.

Rev 2 amendments from rev 1, ordered by Codex priority:
1. §3 — corrected Galois connection statement.
2. §6 — explicit `project_B(⊥)=⊥`, vocabulary-bound proof corrected.
3. §7 — widening policy changed to feedback-vertex-set (loop headers + irreducible SCC headers).
4. §8 — semantic "affected paths" kill visitor added; fixes an existing unsoundness in current code for subpath writes (Codex finding #4).
5. §9 — `Lattice` refactored from interface to struct-of-funcs (no adapter type).
6. §10–§13 — acceptance criteria realigned: propagation-level termination, kill-soundness, determinism. Raw-`Meet` boundedness removed from Condition criteria.

Author intent (Forge journal seq 304): make this checker a real abstract interpreter, not multi-domain fixed-point iteration with informal soundness.

---

## 1. Concrete semantics being abstracted

At program point `p`, the **collecting semantics** is the set of concrete program states `S_p ⊆ State` from which execution can reach `p`. The path-condition abstraction approximates `S_p` by a logical formula in the language of `Constraint` literals (Truthy / Falsy / IsNil / NotNil / HasType / NotHasType / HasField / FieldEquals / FieldNotEquals / FieldEqualsPath / IndexEquals / IndexNotEquals).

For any single source program, the literal set is finite (each literal is determined by source-visible paths, type tags, and constant operands). The vocabulary is bounded **per program**, not globally.

---

## 2. Carrier

DNF over `Constraint` literals. Concretely:

```
Condition := (D₁ ∨ D₂ ∨ … ∨ Dₙ)        n ≥ 0
Dᵢ        := (L₁ ∧ L₂ ∧ … ∧ Lₖᵢ)        kᵢ ≥ 0
Lⱼ        := one of the Constraint literal types
```

Special elements:
- `Bottom` = `FalseCondition` = zero disjuncts. Concretization: ∅.
- `Top` = `TrueCondition` = one empty disjunct. Concretization: full state space.

Canonicalization (already in `normalizeCondition`): within each disjunct, literals are sorted by hash and deduplicated; subsumed disjuncts are removed in the sound direction only (drop `(A ∧ B)` when `A` is also present — never drop `A` to keep `(A ∧ B)`); the resulting DNF is the canonical representative of its equivalence class under those structural rules.

Rationale for keeping DNF (vs F2 BDD / F3 predicate abstraction): downstream consumers iterate the disjunct/conjunct structure directly. Replacing the carrier forces them through a query API or back to DNF at the boundary — moving the blowup, not eliminating it. Codex design step F consult recommended F1 corrected; this design realizes that.

---

## 3. Galois connection (corrected per Codex)

For a single source program, the finite literal vocabulary `P = Lit(program)` defines a propositional logic. The abstract domain is the lattice of DNF formulas over `P` modulo the structural normalization of §2.

```
γ : Condition → 2^State
γ(⊥)                    = ∅
γ((L₁ ∧ … ∧ Lₖ))         = { σ ∈ State : σ ⊨ L₁ ∧ … ∧ σ ⊨ Lₖ }
γ(D₁ ∨ … ∨ Dₙ)           = ⋃ᵢ γ(Dᵢ)
γ(⊤)                    = State
```

The analysis transfers must be **sound over-approximations under γ**:

```
For every transfer T : Condition → Condition modeling a source step,
for every Condition c, the concrete states reachable after the step
from γ(c) are a subset of γ(T(c)).
```

There is no algorithmically-constructed `α : 2^State → Condition` in this design; the analysis never abstracts from arbitrary state sets to formulas. It abstracts only **transitively**: starting from the input condition at each point (a Condition value), each transfer produces another Condition. Soundness reduces to per-transfer over-approximation under γ, proved per-transfer (§8) and preserved under join, meet, and widen (§5, §6).

This avoids the rev-1 mistake of claiming an `α` that was both "chosen by the analysis" and "the residual" — these are incompatible. The honest statement is: γ defines the soundness contract; the analysis is sound iff every transfer satisfies it.

---

## 4. Partial order, equality

```
a ⊑ b   iff   γ(a) ⊆ γ(b)   iff   a ⇒ b   (implication)
a = b   iff   γ(a) = γ(b)   iff   a ⇔ b
```

Implementation:
- `Equal(a, b) := a.Equals(b)` — structural after normalization. Equates exactly the canonical-equivalent formulas, not all logically-equivalent ones.
- `LessOrEq(a, b) := b.Subsumes(a)` — syntactic implication; sound, incomplete.

Incompleteness consequences (per Codex Q1):
- `Truthy(x)` semantically implies `NotNil(x)`, but structural `Subsumes` does not see that. Conditions equivalent under this implication may be incomparable structurally.
- **Direction of harm**: precision loss and unnecessary widening, NOT under-approximation. The fixpoint terminates at a sound over-approximation either way; only precision is affected.
- **Fix is local**: future enrichment of canonicalization can absorb such implications (e.g. canonicalize away a `NotNil(x)` literal when a `Truthy(x)` literal is also present in the same disjunct). Out of scope for this pass — the goal is termination, not precision-perfect implication.

---

## 5. Bottom, Top, Join, Meet

```
Bottom() := FalseCondition()          // zero disjuncts
Top()    := TrueCondition()           // single empty disjunct
Join(a, b) := Or(a, b)                // disjunct set union, then normalize
Meet(a, b) := And(a, b)               // disjunct cross product, then normalize
```

Algebraic properties (verified by `lattice.LawSuite` on a multi-shape sample): `Or` is monotone, commutative, associative, idempotent; `And` is monotone, commutative, associative, semantically idempotent after canonicalization.

Per Codex Q2: subsumed-disjunct removal is **sound only in the direction** "drop `(A ∧ B)` when `A` is already present" — never the reverse. Current implementation in `condition.go:953` orders by shorter disjunct first and checks subset containment, aligning with the sound direction. The law harness pins this against the multi-disjunct sample.

Per Codex's representational-blowup observation: `And` semantically idempotent ≠ representationally bounded. The widening (§6) addresses the unbounded growth at the worklist iterate, not inside `And`. The previously-added "Meet representational-bound" check in the law harness conflicts with this design and is removed in rev 2 (see §10/§13).

---

## 6. Widening operator (Cousot projection widening, rev 2)

### 6.1 Operator definition

```
Lit(c) := { L : L is a literal appearing in some disjunct of c }
         For c = ⊥: Lit(⊥) = ∅
         For c = ⊤: Lit(⊤) = ∅

project_B(c) :=
  if c = ⊥ : ⊥                            // explicit per Codex Q3
  if c = ⊤ : ⊤
  for each disjunct D = (L₁ ∧ … ∧ Lₖ) of c:
    D' := (Lⱼ : Lⱼ ∈ B)                   // retain only literals in B
    if D' is empty: this disjunct projects to ⊤; the whole result is ⊤
  output := normalized DNF of the retained disjuncts (no ⊤ shortcut hit)

Widen(prev, next) :=
  if prev = ⊥ : next
  if next = ⊥ : prev
  if prev = ⊤ : prev
  if LessOrEq(next, prev) [prev ⊒ next] : prev
  B := Lit(prev)
  p := project_B(next)
  Or(prev, p)
```

Implementation discipline (Codex Q3): the projection MUST NOT use `FromDisjuncts(nil)` for the empty-DNF case; that function returns `⊤` for an empty input. The ⊥ case is handled by the explicit `if c = ⊥ : ⊥` branch in `project_B`.

### 6.2 Soundness — `prev ⊑ Widen(prev, next)` and `next ⊑ Widen(prev, next)`

- `prev ⊑ Or(prev, p)` by the upper-bound property of Join.
- `next ⊑ project_B(next)`: for any disjunct `D = (L₁ ∧ … ∧ Lₖ)` of `next` and its projection `D' = (Lⱼ : Lⱼ ∈ B)`, `D ⇒ D'` since dropping conjuncts weakens; by distribution, `next ⇒ p`. The explicit `project_B(⊥) = ⊥` preserves `⊥ ⊑ ⊥`. Therefore `next ⊑ p ⊑ Or(prev, p) = Widen(prev, next)`. □

### 6.3 Termination — ascending chains under Widen are eventually stationary

For any monotone transfer `f`, the chain `s₀ = ⊥, sᵢ₊₁ = Widen(sᵢ, f(sᵢ))` stabilizes:

- Up to the first iteration where the `prev ⊒ next` early-exit does not fire AND `s₀ = ⊥` no longer holds, the iterates may have arbitrary vocabulary.
- The first such iteration produces `s* = Or(prev, project_Lit(prev)(f(prev)))`. The literals in `s*` are a subset of `Lit(prev)`.
- From that point on, every subsequent application of `Widen(sᵢ, f(sᵢ))` either (a) returns `sᵢ` unchanged via the `prev ⊒ next` branch, or (b) re-projects `f(sᵢ)` onto `Lit(sᵢ) ⊆ Lit(s*)` and Or-s. The vocabulary cannot grow.
- The set of normalized DNFs over the finite vocabulary `Lit(s*)` is finite. Therefore the chain enters a finite sub-lattice and stabilizes. □

The "finite normalized DNFs over a finite vocabulary" replaces the rev-1 `2^|B|` figure (Codex Q3, Q10.2): the precise bound for normalized DNFs is below `2^|B|` but the engineering claim "finite, hence terminating" is what matters.

### 6.4 Edge cases (per Codex Q5)

- `prev = A ∧ B`, `next = (A ∧ B ∧ C) ∨ (A ∧ B ∧ D)`: `prev.Subsumes(next)` is true → returns `prev`. No widening fires.
- `Lit(prev) ∩ Lit(next) = ∅`: `B = Lit(prev)`, projecting empties every disjunct of `next`, projection is `⊤`. Result is `Or(prev, ⊤) = ⊤`. Sound, very imprecise. This is the worst case; the feedback-vertex-set widening policy in §7 minimizes how often it fires.
- `prev = ⊤`: returns `⊤` via the explicit branch. `Lit(⊤) = ∅`; if the branch were missing, the projection of any non-`⊥` `next` would also produce `⊤`.
- `prev = ⊥`: returns `next` via the explicit branch.
- `next = ⊥`: returns `prev` via the explicit branch.

### 6.5 What the widening intentionally does NOT do

- Does **not** drop disjuncts. Dropping disjuncts moves `next` toward `⊥` (smaller concretization) — wrong direction, would be unsound.
- Does **not** introduce a numeric `dnfDisjunctCap`. The vocabulary `B = Lit(prev)` is the principled bound; derived from data, not policy.
- Does **not** apply inside `Meet` / `And` / `reinforceLoopPreheader`. Those stay exact transfer logic. Widening is applied exactly once per worklist visit at the state-update site (§7).

---

## 7. Worklist integration boundary (corrected per Codex Q6)

This section specifies WHERE in `propagate.Propagate` the widening operator is called. It is **not** part of the Condition domain proper.

### 7.1 Feedback vertex set

Cousot's classical algorithm widens only at a "feedback vertex set": a set of CFG points such that every cycle in the CFG contains at least one element. Marked loop headers plus one chosen point per nontrivial strongly connected component form an FVS for any CFG.

In the current CFG, `Node.LoopPreheaderSet` marks every structured-loop header (for / while / repeat). Lua's control flow is mostly structured; `goto` and `break` introduce edges but rarely produce irreducible CFGs.

The widening set for design step F is:
```
FVS := { p ∈ CFG : p.LoopPreheaderSet ∨ p is the header of a non-loop SCC }
```

For the second clause, an SCC pass over the CFG identifies any non-loop cycles (typically zero for well-formed Lua). The header of each such SCC is the point with smallest RPO index.

### 7.2 Per-point widen count

```
visitCount[p]  := number of worklist-visits at p that produced a strict state change
loopHeaderWideningThreshold := 3
```

### 7.3 Integration

```
for each point p in worklist (ordered RPO):
  incoming := computeConditionAtPoint(p)
  oldCond  := pointConditions[p]
  candidate := Or(oldCond, incoming)
  if p ∈ FVS and visitCount[p] >= loopHeaderWideningThreshold:
    next := Domain.Widen(oldCond, candidate)
  else:
    next := candidate
  if not Equal(next, oldCond):
    pointConditions[p] := next
    visitCount[p] += 1
    enqueue successors
```

### 7.4 "All points after K" demoted to fallback (Codex Q6)

Rev 1 widened at any point exceeding K state-changing visits. Codex showed this is sound but over-widens acyclic high-fan-in joins under unlucky RPO ordering, destroying precision. Rev 2 widens only at FVS; visiting an acyclic point K times is normal worklist behavior and triggers no widening.

If a CFG pathology not caught by the FVS computation causes non-termination, the harness's per-fixture deadline (commit 930068c9) catches it and the design is amended to extend the FVS. There is no emergency all-point fallback in the production code.

---

## 8. Transfer functions: kill, edge refinement (corrected per Codex Q8 — existing soundness gap)

### 8.1 Edge refinement (unchanged)

At edge `(p → q)` with edge condition `e`, the condition at `q` from this edge is `pointConditions[p] ∧ e`. Existing `applyEdgeCondition`; this design does not change it.

### 8.2 Constraint kill on assignment (corrected — fixes existing unsoundness)

**Codex finding (Q8)**: the current kill in `propagate.computeConditionAtPoint` drops literals via `VisitPaths` which visits only the *root path* of each constraint. For a constraint like `FieldEquals{Target: x, Field: "kind"}`, the visited path is `x` — not `x.kind`. So:
- Root assignment `x = …` → drops the literal (current behavior, correct).
- **Field assignment `x.kind = …` → does NOT drop the literal** (current behavior, unsound).

Same gap for `HasField`, `FieldNotEquals`, `FieldEqualsPath`, `IndexEquals`, `IndexNotEquals`.

**Fix**: introduce a `SemanticAffectedPaths(c Constraint) []Path` visitor that exposes the **full semantic access path(s)** of a constraint, not just its root:
```
Truthy{p}                 → [p]
Falsy{p}                  → [p]
IsNil{p}                  → [p]
NotNil{p}                 → [p]
HasType{p, T}             → [p]
NotHasType{p, T}          → [p]
HasField{p, f}            → [p, p.f]                     // both: presence of f at p, and read of p.f
FieldEquals{p, f, lit}    → [p, p.f]
FieldNotEquals{p, f, lit} → [p, p.f]
FieldEqualsPath{p, f, q}  → [p, p.f, q]
IndexEquals{p, k, lit}    → [p, p[k]]                    // when k is a literal segment
IndexNotEquals{p, k, lit} → [p, p[k]]
```

Kill rule (rev 2, corrected during implementation per agent-detected precision regression): assignment to path `w` kills any literal `L` such that one of `SemanticAffectedPaths(L)` has `w` as a (non-strict) prefix. That is, `w` must be `L`'s access path or an ancestor of it. The descendant case is intentionally NOT killed because the literal's full read path is already exposed by `SemanticAffectedPaths`. Example: a literal `FieldEquals{x, "kind", "a"}` exposes paths `[x, x.kind]`; an assignment to `x.kind` matches because `x.kind` is a (non-strict) prefix of the literal's `x.kind` path — soundness covered. An unrelated assignment to `x.value` does NOT kill the literal because `x.value` is neither a prefix of `x` nor of `x.kind` — precision retained. This is the standard path-mod / Andersen-style kill formulation.

The semantic visitor is added to `types/constraint/path_visit.go`. `propagate.computeConditionAtPoint` uses it in the kill step.

### 8.3 Closure / cross-procedural

Unchanged from rev 1: existing effects propagation tracks per-function side effects; conditions referencing paths the callee writes are killed at the call site. Soundness of the kill is preserved under widening because widening only weakens (γ-monotone).

---

## 9. Public API surface (corrected per Codex Q9 — Lattice refactor)

### 9.1 `types/lattice` refactor

The current `types/lattice/lattice.go` defines `Lattice[T]` as an **interface**. The user has explicitly forbidden adapter types. The only way to expose a domain's lattice value without an adapter is to make `Lattice[T]` a **struct of function fields** (Apron-style; consistent with the user-approved earlier session sketch that was reverted).

```go
type Lattice[T any] struct {
    Bottom   func() T
    Top      func() T
    Equal    func(a, b T) bool
    LessOrEq func(a, b T) bool
    Join     func(a, b T) T
    Meet     func(a, b T) T
    Widen    func(prev, next T) T
}
```

`LawSuite[T]` calls these fields directly.

### 9.2 Per-domain wiring (Condition)

```go
package constraint

// existing carrier and primitives unchanged

func (c Condition) WidenAgainst(next Condition) Condition  // NEW

// SemanticAffectedPaths exposes the semantic access path(s) of a Constraint
// for the kill-on-assignment transfer (§8.2 — fixes an existing unsoundness).
func SemanticAffectedPaths(c Constraint) []Path            // NEW

// Single value exposing the domain to the harness and to the worklist.
var Domain = lattice.Lattice[Condition]{
    Bottom:   FalseCondition,
    Top:      TrueCondition,
    Equal:    func(a, b Condition) bool { return a.Equals(b) },
    LessOrEq: func(a, b Condition) bool { return b.Subsumes(a) },
    Join:     Or,
    Meet:     And,
    Widen:    func(p, n Condition) Condition { return p.WidenAgainst(n) },
}
```

No adapter type. The struct-of-funcs IS the agreement; nothing wraps anything.

---

## 10. Test plan (corrected per Codex Q10)

### 10.1 Lattice-law conformance — `condition_lattice_test.go`

`lattice.LawSuite[Condition]` with a sample covering EVERY Constraint kind (Truthy, Falsy, IsNil, NotNil, HasType, NotHasType, HasField, FieldEquals, FieldNotEquals, FieldEqualsPath, IndexEquals, IndexNotEquals), duplicate literals, subsumed disjuncts, contradictory pairs (e.g. `IsNil(x) ∧ NotNil(x)`), and the empty-projection / TRUE / FALSE cases.

### 10.2 Widening laws — `widen_test.go` (NEW)

- **Soundness**: for sample pairs, both `prev.Subsumes(Widen(prev,next))` (treating `Subsumes` as `⊑`-witness) and `next.Subsumes(Widen(prev,next))` hold modulo direction.
- **Idempotency on stable chains**: `Widen(c, c) = c`.
- **Vocabulary fixing across multi-iteration chain**: starting from a fixed `prev` and a transfer that adds disjuncts with FRESH literals each iteration, after one widening every subsequent state's literals ⊆ `Lit(prev)`.
- **Empty projection edge case**: `Widen(FromConstraints(a), FromConstraints(b))` with disjoint literals = `TrueCondition`; `Widen(⊥, c) = c`; `Widen(c, ⊥) = c`; `Widen(⊤, c) = ⊤`.
- **Unsound-direction regression-lock**: a fixture asserting that the implementation does NOT drop disjuncts. Implemented as a property test: for all (prev, next), every disjunct of `Widen(prev, next)` has a literal subset of some disjunct of `Or(prev, next)`.

### 10.3 Kill soundness across all Constraint kinds — `kill_test.go` (NEW)

For each pair (constraint kind, assignment shape):
- `x = …`: every constraint mentioning `x` (root or any descendant path) is killed.
- `x.kind = …`: every constraint whose `SemanticAffectedPaths` contain `x.kind` or an ancestor/descendant — **including** `FieldEquals{x, kind, lit}` — is killed.
- `x[k] = …` with literal `k`: every constraint whose `SemanticAffectedPaths` contain `x[k]` — including `IndexEquals{x, k, lit}` — is killed.
- After widening at a header, the final post-update condition at an assignment point must contain no affected literals. (Codex Q10.3: stronger than "kill commutes with widening".)

### 10.4 Synthetic-CFG termination — `propagate_synthetic_test.go` (NEW)

For each divergent fixture pattern (loop-preheader reinforcement with multi-disjunct preheader; multi-backedge SCC; nested loops): a synthetic CFG that reproduces the structural pattern. Assert:
- termination in O(|CFG| × K) worklist visits;
- determinism under shuffled RPO;
- precision retention on the corresponding non-loop CFG.

NO fixture-suite invocation in this test (per process gate).

### 10.5 Acyclic high-fan-in regression — `propagate_acyclic_test.go` (NEW)

A synthetic CFG with one merge point of N predecessors. Assert: widening does NOT fire because the merge point is not in the FVS. (Codex Q6: rev 2 policy must not over-widen acyclic high-fan-in.)

---

## 11. Documentation deliverables

- `DOMAIN_DESIGN.md` (this file).
- Package header doc-comments in `types/constraint/`, `types/lattice/`, `types/flow/propagate/` referencing this file.
- Per-function doc on `WidenAgainst`, `SemanticAffectedPaths`, `Domain`, `projectOntoVocabulary` (internal), stating their soundness obligations.
- Forge journal entry on landing referencing seq 304.

---

## 12. Out of scope

- design step D Kildall refactor (the worklist redesign).
- Other abstract domains.
- Stdlib precise typing.
- Bilateral `MU ↔ nil` metatable hack.
- Improving syntactic `Subsumes` toward semantic implication (future precision work).

---

## 13. Acceptance criteria (corrected per Codex Q11)

Condition is closed iff:

- `lattice.LawSuite[Condition].Run` passes on the §10.1 sample.
- `widen_test.go` passes all §10.2 cases including the unsound-direction regression lock and empty-projection edge cases.
- `kill_test.go` passes all §10.3 cases — including SUBPATH writes (`x.kind = …`, `x[k] = …`). This locks the soundness fix.
- `propagate_synthetic_test.go` passes; termination on each known divergent fixture's structural pattern is reproduced and stabilizes.
- `propagate_acyclic_test.go` passes — widening does not fire on acyclic high-fan-in joins.
- Determinism under shuffled RPO holds.
- `go test ./types/constraint/... ./types/lattice/... ./types/flow/propagate/...` is green.
- `go test ./compiler/check/... ./types/...` (Go test suite, no fixtures) regresses no tests vs HEAD (only the pre-existing `tests/errors` 90s hang remains).
- No fixture-suite invocation appears in any agent or test driver for this milestone.
- The lattice law harness's prior raw-Meet representational-bound check (added in rev 1) is REMOVED — it conflicted with this design's premise (widening happens at the worklist, not inside Meet). Removal is part of the Condition close.

---

## 14. Implementation plan

ONE child agent, scoped strictly to:
- `types/constraint/`
- `types/lattice/`
- `types/flow/propagate/`

The agent receives:
- this document (`DOMAIN_DESIGN.md`)
- Codex's review (`.codex-out-condition-design.txt`)
- the design step F variant choice (`.codex-out-step-f.txt`)

The agent implements §3–§9, the tests in §10, and the docs in §11. No code outside the three named directories. **No fixture-suite invocation in any test run.** No other-domain work.

The agent reports per-criterion pass/fail in §13 and stops. On pass, I commit and journal. On any fail, the agent stops and reports the gap; I refine this document, re-verify with Codex, re-dispatch.
