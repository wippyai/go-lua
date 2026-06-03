# Numeric Abstract Domain — Design Document (rev 2)

Status: rev 3 (Codex re-rejected rev 2 with 4 amendments — `var Domain` name collision, modular Join via congruence hull, Widen-nil triage policy, Equals honesty — all folded). Rev 1 was rejected for "wiring-only" framing; rev 2 was rejected for the four issues above. Rev 3 is the corrected design.

Background: `.codex-out-numeric.txt` (rev 1 rejection), `.codex-out-numeric-rev2.txt` (rev 2 rejection).

The existing `numeric.State` does NOT implement a sound lattice today. Three real algebraic bugs need fixing before the Lattice contract can be wired:

1. `Join` does interval INTERSECTION, not interval HULL. `Join([5,10], [5,12])` returns `[5,10]`; should return `[5,12]` (the LUB). `domain.go:112` documents the correct LUB shape (`lower=min, upper=max`) but `state.go:199` disagrees. This makes `LessOrEq := Join(a,b).Equals(b)` semantically wrong everywhere.
2. `Widen` drops whole intervals when ANY bound moves, instead of selective Cousot widening (keep stable bounds; drop moved bounds to ±∞ / domain edges).
3. `Widen(nil-Top, constrained)` returns the constrained state — doesn't over-approximate `Top`.

Scope: fix all three bugs in `numeric.State`, then wire `Domain`, then test. Comparable to Condition design step F in scope.

---

## 1. Concrete semantics being abstracted

Per rev 1 §1 — unchanged. `numeric.State` abstracts integer-variable runtime values via interval bounds, modular residues, difference constraints, length references, and length bounds.

`State == nil` represents Top (no constraints). `State.unsat == true` represents Bottom.

---

## 2. Carrier

Per rev 1 §2 — unchanged. `*numeric.State`.

---

## 3. Galois connection

```
γ(nil)       = full state space             // Top
γ(unsat)     = ∅                            // Bottom
γ(state)     = { σ : every bound, residue, diff-constraint, length-fact in state holds under σ }
```

Soundness contract: every transfer must over-approximate the concrete states reachable after the source step. Existing transfers (`atom_applier.go`, `theory.go`) are assumed sound; this design fixes only the Join/Widen surface.

---

## 4. Partial order, equality

```
a ⊑ b   iff   γ(a) ⊆ γ(b)   iff   a has at least the constraints of b
                                 (more constraints = lower in lattice = more restrictive = smaller concretization)
```

After the Join fix, `LessOrEq(a, b) := Join(a, b).Equals(b)` will hold the standard join-induced order: `a ⊑ b` iff `a ⊔ b = b`. Until Join is fixed, this formulation is broken.

Equality: existing `State.Equals(other)` is **structural carrier equality**, not semantic γ-equality. Two structurally-distinct states may have equal concretizations (e.g. one stores `x ≡ 0 mod 4` and another stores `x ≡ 0 mod 4` after simplifying from `gcd(4,8,0)=4`). The LawSuite uses structural equality; the design does NOT claim Equals implies γ-equality. Soundness of the lattice contract holds under structural equality because both `LessOrEq` (= `Join(a,b).Equals(b)`) and the algebraic laws are stated in carrier-Equal terms.

---

## 5. Required algebra fixes (the substance of this design)

### 5.1 Fix Join to be true LUB

Current `Join` in `state.go:184-270` intersects intervals: for variable `x` present in both `a` and `b` with bounds `[la, ua]` and `[lb, ub]`, the result has `x ∈ [max(la, lb), min(ua, ub)]`.

LUB requires the HULL: `x ∈ [min(la, lb), max(ua, ub)]`.

Same correction applies to `lenBounds` (currently intersection per `state.go` widenLengthInterval). LUB = interval hull.

Modular constraints (rev 3 per Codex — "drop when different" is sound but not LUB; correct LUB is the congruence hull):
- For `x ≡ r₁ mod m₁` joined with `x ≡ r₂ mod m₂`, compute `g = gcd(m₁, m₂, |r₁ − r₂|)`.
- If `g == 1`, drop the modular constraint (no useful shared congruence).
- Otherwise keep `x ≡ normalize(r₁, g) mod g`, where `normalize(r, g) = r mod g`.
- If the key is absent in either state, drop (LUB is the weaker = no constraint).
- Example: `x ≡ 0 mod 4 ⊔ x ≡ 0 mod 2 = x ≡ 0 mod 2` (because gcd(4, 2, 0) = 2).

Difference constraints: `x - y ≤ c₁` joined with `x - y ≤ c₂` — LUB is `x - y ≤ max(c₁, c₂)` (weaker bound). If one is absent, drop the constraint.

Length references (`lenRefs`): `x ≤ len(arr) + off₁` joined with `x ≤ len(arr) + off₂` — LUB is `x ≤ len(arr) + max(off₁, off₂)`. Different `Array` keys → drop. If absent in one → drop.

Top (nil) handling: `Join(nil, x) = nil` (Top is absorbing in join). `Join(x, nil) = nil`. Per Codex Q2 the current behavior `Join(nil, x) = x` is the bug.

Bottom (unsat) handling: `Join(Bottom, x) = x` (Bottom is identity in join). `Join(x, Bottom) = x`. Verify current behavior.

### 5.2 Fix Widen to be textbook Cousot

Current `Widen` at `state.go:277-343` drops the entire bound for `x` if `prev[x] != next[x]`. Textbook Cousot widening:

```
Widen(prev_interval, next_interval):
  lower = if next.Lower < prev.Lower then -∞ (domain min) else prev.Lower
  upper = if next.Upper > prev.Upper then +∞ (domain max) else prev.Upper
```

Stable lower bound retained, moved lower drops; same for upper. Independently per bound.

For domain min/max use `math.MinInt64 / math.MaxInt64` (the existing infinity sentinels per `state.go`).

If `prev[x]` is absent and `next[x]` is present, the new bound is unstable — drop. (This is "missing in prev" = unstable per the standard rule.)

Same per-bound logic for `lenBounds`.

For `lenRefs` and modular and difference constraints: keep if exactly equal across iterations (these are discrete facts; either stable or not).

Top handling: `Widen(nil-Top, x) = nil-Top` (Top over-approximates anything). `Widen(x, nil-Top) = nil-Top`. Per Codex Q1 the current behavior `Widen(nil, constrained) = constrained` is the bug.

Bottom handling: `Widen(Bottom, x) = x` (Bottom widens to anything). `Widen(x, Bottom) = x` (Bottom adds no information).

### 5.3 Verify Equals after the fixes

`State.Equals` may need a sanity check: after the Join fix, two structurally-distinct states that join to the same `b` should be Equal under `Equals`. Re-read `state.go:892` to confirm. The existing implementation may already be correct; if not, fix forward.

---

## 6. Operations after fixes

```
Bottom() *State           // existing — Bottom() returns unsat state
Top() *State              // NEW — returns nil
Join(a, b *State) *State  // FIXED per §5.1
Widen(prev, next *State) *State  // FIXED per §5.2
Equals(a, b *State) bool  // existing; verify per §5.3
LessOrEq(a, b *State) bool  // NEW — Join(a, b).Equals(b)
```

`Meet`: still `nil` per Codex Q3 acceptance. The natural binary meet is constraint conjunction, but the codebase uses transfer-specific `ApplyConstraint`, not a generic `Meet(a, b)`. Leaving Meet nil is honest about the consumer surface.

---

## 7. Public API surface (no adapter)

```go
package numeric

// Existing — fixed per §5
func Bottom() *State
func Join(a, b *State) *State          // FIXED
func Widen(prev, next *State) *State   // FIXED
func (s *State) Equals(other *State) bool
func (s *State) IsUnsat() bool
func (s *State) IsTop() bool

// NEW
func Top() *State { return nil }
func LessOrEq(a, b *State) bool { return Join(a, b).Equals(b) }

// NEW — single variable, no adapter
var StateDomain = lattice.Lattice[*State]{
    Bottom:   Bottom,
    Top:      Top,
    Equal:    func(a, b *State) bool { return a.Equals(b) },
    LessOrEq: LessOrEq,
    Join:     Join,
    Meet:     nil,
    Widen:    Widen,
}
```

`Domain` in a new file `domain_lattice.go`.

---

## 8. Tests (per Codex Q4/Q5)

`types/flow/numeric/domain_lattice_test.go` (NEW):

Sample (Codex-expanded):
- `Bottom()`
- `Top()` (nil)
- A state with `x ∈ [5, 10]`.
- A state with `x ∈ [5, 12]`.
- A state with `x ∈ [3, 10]`.
- A state with `x ∈ [3, 12]`.
- A state with `len(arr) ∈ [5, 10]`.
- A state with `len(arr) ∈ [5, 12]` (decreasing lower in widening test).
- A state with `x - y ≤ 0`.
- A state with `x - y ≤ 5` (same key, different constants).
- A relation+bounds state whose inconsistency is only theory-solver visible.
- A state with `x ≡ 0 mod 3`.
- A state with `x ≡ 1 mod 3` (different residue).
- A state with combined bounds+diff+modular constraints.

`TestNumericLattice_Laws`: applies `lattice.LawSuite[*State]`. All join, partial-order, widening laws. Meet-side laws skipped (Meet=nil).

`TestNumericLattice_JoinTakesIntervalHull`:
- `Join([5,10], [5,12]) = [5,12]` (was `[5,10]` per Codex).
- `Join([3,10], [5,12]) = [3,12]`.
- `Join(Top, x) = Top`.
- `Join(Bottom, x) = x`.

`TestNumericLattice_WidenIsCousotIntervalWidening`:
- `Widen([5,10], [5,12])` keeps lower=5, drops upper (or sets to MaxInt). Verify per the fix.
- `Widen([5,10], [3,10])` drops lower (or sets to MinInt), keeps upper=10.
- `Widen([5,10], [3,12])` drops both.
- `Widen([5,10], [5,10])` returns equal state (stable).
- `Widen(Top, x) = Top`.
- `Widen(x, Top) = Top`.
- `Widen(Bottom, x) = x`.

`TestNumericLattice_LessOrEqMatchesGamma`:
- `LessOrEq([5,10], [5,12])` = true (more restrictive state is below less restrictive).
- `LessOrEq([5,12], [5,10])` = false.
- `LessOrEq(Bottom, anything) = true`.
- `LessOrEq(anything, Top) = true`.

`TestNumericLattice_ConstraintConjunctionInconsistency`: a state with both `x ∈ [10, 20]` and `y ∈ [10, 20]` and `x - y ≤ -5` and `x - y ≥ 5` should be Bottom after constraint application. (Tests that Bellman-Ford detects unsat through the theory solver as expected; not a Lattice law per se, but pinpoints the existing soundness depends on this being correct.)

---

## 9. Verification

- `go build ./...` clean.
- `go test -count=1 ./types/flow/numeric/... ./types/lattice/...` all green.
- `go test -count=1 ./compiler/check/... ./types/...` no regressions vs HEAD.
- No fixture-suite invocation.

Caveats (rev 3 per Codex Q3):

- Fixing `Join` from intersection to hull may change downstream behavior in `solver.go::computeNumericStateAt` (predecessor merge — hull is the right semantics here, no problem expected) and in `types/flow/numeric/domain.go:87` (calls `numeric.Widen(oldState, rawState)` — see below).

- Fixing `Widen(nil, x) = nil-Top` is lattice-correct but high-risk: `types/flow/numeric/domain.go:87` may use `oldState == nil` to mean "uninitialized seed", not "Top". If the lattice-correct widening returns Top for nil-prev, the flow engine may fail to seed numeric facts on the first visit. **Triage policy: STOP and report if the non-regression gate exposes this**. Do NOT special-case the lattice Widen back to the buggy behavior — the flow engine must distinguish uninitialized state from lattice Top. The fix is outside this agent's scope.

- Any test regressing in `compiler/check/...` after the algebra fix must be triaged: tests depending on the buggy intersection/widening semantics get FIXED to assert the corrected behavior; downstream consumers using `Join` for a meet-like purpose get STOPPED+REPORTED.

---

## 10. Acceptance criteria

- `LawSuite[*State].Run` passes on §8 sample.
- The 4 domain-specific tests pass.
- No regressions in the full Go test suite (no fixtures). Regressions from the Join fix must be triaged per §9 caveat.
- No adapter type.
- `Domain` is one declaration.

---

## 11. Out of scope

- LengthBound separate domain (subsumed by Numeric per investigation; one fewer to migrate).
- Theory-solver transfer-monotonicity (task #24, design step C).
- design step D Kildall refactor.
- The stale `LengthBound` mention in `types/lattice/lattice.go:4` (small doc cleanup, fold into commit).

---

## 12. Implementation plan

ONE child agent, scoped strictly to:
- `types/flow/numeric/`
- `types/lattice/` (only for the stale doc comment cleanup)

Agent steps:
1. Fix `Join` per §5.1 — interval hull, correct Top/Bottom handling, fix all five constraint categories (bounds, lenBounds, modular, relations, lenRefs).
2. Fix `Widen` per §5.2 — Cousot per-bound stability check; same five categories.
3. Verify `Equals` per §5.3; fix if needed.
4. Add `Top()` and `LessOrEq` to `state.go`.
5. Add `domain_lattice.go` with `Domain` variable.
6. Add `domain_lattice_test.go` with sample + LawSuite + 4 domain-specific tests.
7. Triage any regressions in scoped tests + non-regression gate per §9 caveat.
8. Cleanup the stale `LengthBound` mention in `types/lattice/lattice.go:4`.
9. Report per acceptance criterion.

If any caveat-triage step requires touching code outside `types/flow/numeric/` or `types/lattice/`, STOP and report — don't expand scope without orchestrator approval.
