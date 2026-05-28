# PathPresence Abstract Domain — Design Document

Status: DRAFT for Codex review.
Scope: wire the existing `pathPresence` type (in `types/flow/path_presence.go`) to the `lattice.Lattice[pathPresence]` contract. Third domain after Condition (`93d2170b`) and AbstractValue (`126c4322`).

This is the smallest possible domain redo: a 4-element finite lattice with the trivial widening (Widen = Join). The point is not to discover bugs (there are unlikely to be any here) but to lock the contract and demonstrate the per-domain workflow scales to trivial finite-height cases.

---

## 1. Concrete semantics being abstracted

At a path-key key `K` at program point `p`, the path-presence value abstracts the runtime presence of a value at that key in concrete states reaching `p`:
- present in every reaching state
- absent in every reaching state
- present in some and absent in others
- unknown (no information / not yet computed)

This is used by `s.presenceAtPoint`, `s.setValuePresence`, `s.setMutablePresence`, and downstream by `projectPathPresence` to project presence into the type domain (e.g. project a `T?` to `T` under known-present, or to `nil` under known-absent).

---

## 2. Carrier

```go
type pathPresence uint8

const (
    pathPresenceUnknown pathPresence = iota
    pathPresencePresent
    pathPresenceAbsent
    pathPresenceMaybe
)
```

Four elements, total. Unexported.

Implication for the wiring: the `Domain` variable must live INSIDE `types/flow` because `pathPresence` is unexported. The `lattice.Lattice[T]` contract is generic and accepts an unexported type as the type parameter (since the consumer is local).

No exporting is required.

---

## 3. Galois connection (rev 2 per Codex)

The existing `joinPathPresence` (`types/flow/path_presence.go:34`) treats `Unknown` as the JOIN IDENTITY: `Join(Unknown, x) = x`. That means operationally **Unknown = Bottom**, not Top.

Codex rejected the rev 1 framing of "Unknown = Top" because it contradicted the existing code. Rev 2 keeps the 4-element lattice as it operationally exists; no 5th constant needed:

```
γ(pathPresenceUnknown) = ∅                                   // Bottom — no information yet, the join identity
γ(pathPresencePresent) = states where the key is definitely present
γ(pathPresenceAbsent)  = states where the key is definitely absent
γ(pathPresenceMaybe)   = full state space (Present ∪ Absent) // Top — any possibility admitted
```

This matches operational semantics: `Unknown` means "no information yet", joining it with `Present` learns presence; `Maybe` means "could be either" (least precise non-Bottom). The "Unknown=Bottom, Maybe=Top" reading is the only one consistent with the existing `joinPathPresence` semantics.

Note: γ(Maybe) = γ(Present) ∪ γ(Absent) = the full state space (presence is binary), so γ(Maybe) and what we might "naively call Top" coincide; the carrier still distinguishes them from the empty-concretization Bottom (Unknown).

---

## 4. Partial order (rev 2)

```
Unknown ⊑ everything   (Bottom — no info; the join identity)
Unknown ⊑ Present
Unknown ⊑ Absent
Present ⊑ Maybe        (Top)
Absent  ⊑ Maybe        (Top)
```

Hasse:
```
        Maybe          (Top)
        /   \
   Present  Absent
        \   /
        Unknown        (Bottom)
```

4 elements; longest strict chain `Unknown → Present|Absent → Maybe` has 2 strict steps; finite height 2.

---

## 5. Operations (rev 2 per Codex)

```
Bottom()        = pathPresenceUnknown   (matches existing join-identity behavior)
Top()           = pathPresenceMaybe     (the join of Present and Absent)
Equal(a, b)     = a == b
LessOrEq(a, b)  = Equal(Join(a, b), b)
Join(a, b)      = existing joinPathPresence (verify the table per below)
Meet(a, b)      = new — per the table below
Widen(prev, next) = Join (finite height — chain bound 2)
```

**Join table (must match the existing code, verified per Codex finding 1)**:
```
Unknown ⊔ x       = x                 (Unknown is the join identity)
Maybe ⊔ x         = Maybe              (Maybe is the absorbing element / Top)
Present ⊔ Present = Present
Absent ⊔ Absent   = Absent
Present ⊔ Absent  = Maybe
```

**Meet table (NEW)**:
```
Unknown ⊓ x       = Unknown            (Unknown is Bottom; meet absorbs)
Maybe ⊓ x         = x                  (Maybe is Top; meet is identity)
Present ⊓ Present = Present
Absent ⊓ Absent   = Absent
Present ⊓ Absent  = Unknown            (their GLB is Bottom: no shared concretization)
```

Codex emphasized: keep absorption as a law check even though redundant with explicit tables; catches independent Join/Meet implementation drift cheaply.

---

## 6. Public API surface (no adapter, rev 2)

```go
package flow

// Existing constants — no addition needed (Codex finding: Unknown already
// behaves as Bottom; no 5th element required).
//   pathPresenceUnknown, pathPresencePresent, pathPresenceAbsent, pathPresenceMaybe

// Existing — verify the table matches §5 (Unknown is join identity, Maybe is
// join absorbing element, Present ⊔ Absent = Maybe).
func joinPathPresence(a, b pathPresence) pathPresence

// NEW
func meetPathPresence(a, b pathPresence) pathPresence

// NEW — single variable, no adapter
var pathPresenceDomain = lattice.Lattice[pathPresence]{
    Bottom:   func() pathPresence { return pathPresenceUnknown },
    Top:      func() pathPresence { return pathPresenceMaybe },
    Equal:    func(a, b pathPresence) bool { return a == b },
    LessOrEq: func(a, b pathPresence) bool { return joinPathPresence(a, b) == b },
    Join:     joinPathPresence,
    Meet:     meetPathPresence,
    Widen:    joinPathPresence,
}
```

Why no adapter: the contract IS a struct of function fields. We point the fields at existing/new package functions directly.

Unexported (`pathPresenceDomain`); LawSuite test lives in the same package. No public API change.

---

## 7. Tests

`types/flow/path_presence_lattice_test.go` (NEW):

- `TestPathPresenceLattice_Laws` applies `lattice.LawSuite[pathPresence]` over the 5-element sample (Bottom, Present, Absent, Maybe, Unknown). Asserts every algebraic law.
- `TestPathPresenceLattice_LessOrEqMatchesJoin` asserts `pathPresenceLessOrEq(a, b) == (joinPathPresence(a, b) == b)` for every pair — pins the join-induced order property.
- `TestPathPresenceLattice_MeetMatchesJoinOnComparable` asserts `pathPresenceLessOrEq(a, b) ⇒ meetPathPresence(a, b) == a` — pins meet correctness on comparable pairs.
- `TestPathPresenceLattice_WidenIsJoin` asserts `Widen(a, b) == Join(a, b)` for every pair.

---

## 8. Verification

- `go build ./...` clean.
- `go test -count=1 ./types/flow/... ./types/lattice/...` all green.
- `go test -count=1 ./compiler/check/... ./types/...` regresses no tests vs HEAD.
- No fixture-suite invocation.

The 4×4 = 16 pair-tests + the 5-element LawSuite sample are exhaustive over the finite carrier. No property-test randomness needed.

---

## 9. Out of scope

- NumericRange — separate domain, theory-solver state, needs its own design.
- LengthBound — not currently a separate package.
- Any code outside `types/flow/` and `types/lattice/`.

---

## 10. Acceptance criteria

- LawSuite passes on the 5-element sample.
- The 4 explicit relation tests pass.
- No regressions in the full Go test suite (no fixtures).
- No adapter type.
- Single `pathPresenceDomain` variable; everything else is existing or trivially-extended package primitives.
- All consumers of `pathPresence` still compile (the added Bottom value triggers no switch-exhaustiveness errors because Go doesn't enforce them; verify all switches over `pathPresence` either have a default or handle the new value explicitly if any are exhaustive-by-construction).

---

## 11. Implementation plan

Direct edit (no child agent — surface too small to warrant the overhead). I have already verified the workflow on Condition and AbstractValue; PathPresence is a sub-50-LOC delta.

1. Add `pathPresenceBottom = 4`.
2. Audit every switch over `pathPresence` — confirm each handles the new constant or has a default. Five switches likely (path_presence.go and consumers).
3. Extend `joinPathPresence`: Bottom ⊔ x = x.
4. Add `meetPathPresence`.
5. Add `pathPresenceLessOrEq`.
6. Add `pathPresenceDomain` variable.
7. Add `path_presence_lattice_test.go` with the four tests above.
8. Run scoped tests + non-regression gate.
9. Commit.
