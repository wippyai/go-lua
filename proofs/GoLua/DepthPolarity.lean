/-
Mechanization pilot for obligation B4 / invariants.md Rule 1
("Positive proof is false at depth exhaustion; may-contain remains true").

Go source mirrored:
- analysis/type/subtype/guard.go: `stopDepthPair(sub, super, depth)` returns
  true (stop, and the caller reports false) once `depth > typ.DefaultRecursionDepth`.
  Consumed by `check`/`canWidenTo` in analysis/type/subtype/subtype.go, tested
  by `TestSubtypeDepthExhaustionFailsClosed`
  (analysis/type/subtype/subtype_test.go) and
  `TestValueProofAdmissibleRuntimeCastDepthExhaustionFailsClosed`
  (analysis/domain/value/proof/proof_test.go): a bounded *positive* relation
  (subtype, admissible runtime cast) must answer false, never true, once its
  recursion budget runs out, even when the honest unbounded answer would have
  been true.
- analysis/check/readmodel/api.go: `numericForMayContainNumber(t, depth, active)`
  returns true once `depth > typ.DefaultRecursionDepth`: the dual "may-contain"
  existential query fails open (true = "can't rule it out"), never false, at
  the same exhaustion point.

invariants.md Rule 1 states these as one pairing: a bounded positive relation
returns false at exhaustion; its may-contain dual returns true at exhaustion.
This file proves the soundness consequence of that pairing over a small,
honest abstract model: exhaustion never lets the bounded positive relation
claim a truth the unbounded (exact) relation does not have, and dually,
exhaustion never lets the bounded may-contain relation claim an absence the
unbounded relation does not have.
-/

namespace GoLua.DepthPolarity

/-- A minimal stand-in for a recursive type-graph node: an atomic fact, or a
composite with two substructures. `Term` mirrors the shape both real
traversals walk (record fields for `check`/`canWidenTo`'s universal
conjunction; union members / wrapped types for `numericForMayContainNumber`'s
existential disjunction) without carrying any of the real type-graph detail
that is irrelevant to the depth-exhaustion polarity argument itself. -/
inductive Term where
  | atom (p : Bool)
  | node (l r : Term)

open Term

/-- The exact, unbounded positive answer: a conjunction over the whole term
(mirrors `check`/`canWidenTo`'s universal recursion — every substructure must
satisfy the relation). `Term` is a finite Lean tree, so this recursion is
structurally well-founded on its own; the point of the model is the budget
mechanism below, not term-graph cyclicity. -/
def actualPositive : Term → Bool
  | atom p => p
  | node l r => actualPositive l && actualPositive r

/-- The exact, unbounded may-contain answer: a disjunction over the whole term
(mirrors `numericForMayContainNumber`'s existential recursion). -/
def actualMay : Term → Bool
  | atom p => p
  | node l r => actualMay l || actualMay r

/-- The bounded positive relation. `d` is fuel counted down from the caller's
budget; mirrors `stopDepthPair`'s `depth > typ.DefaultRecursionDepth` check
by returning `false` the instant fuel is exhausted, before even looking at
the current node — the fail-closed half of Rule 1. -/
def boundedPositive : Nat → Term → Bool
  | 0, _ => false
  | _ + 1, atom p => p
  | d + 1, node l r => boundedPositive d l && boundedPositive d r

/-- The bounded may-contain dual. Same fuel discipline, opposite polarity at
exhaustion: `true`, mirroring `numericForMayContainNumber`'s
`depth > typ.DefaultRecursionDepth => true` — the fail-open half of Rule 1. -/
def boundedMay : Nat → Term → Bool
  | 0, _ => true
  | _ + 1, atom p => p
  | d + 1, node l r => boundedMay d l || boundedMay d r

/-- The minimum fuel a term needs before `boundedPositive`/`boundedMay` match
the exact answer: one unit per level, an atom needs at least one unit to be
looked at all. -/
def size : Term → Nat
  | atom _ => 1
  | node l r => 1 + max (size l) (size r)

theorem size_pos (t : Term) : 0 < size t := by
  cases t <;> simp only [size] <;> omega

/-! ### Exactness once the budget suffices

Mirrors the "when `d` sufficient" half of the mapping in the task
description: a budget at least as large as the term's depth reproduces the
exact answer exactly, for both polarities. -/

theorem boundedPositive_exact_of_sufficient :
    ∀ (t : Term) (d : Nat), size t ≤ d → boundedPositive d t = actualPositive t := by
  intro t
  induction t with
  | atom p =>
    intro d hd
    cases d with
    | zero => exfalso; simp only [size] at hd; omega
    | succ e => rfl
  | node l r ihl ihr =>
    intro d hd
    cases d with
    | zero => exfalso; simp only [size] at hd; omega
    | succ e =>
      have hl : size l ≤ e := by simp only [size] at hd; omega
      have hr : size r ≤ e := by simp only [size] at hd; omega
      show (boundedPositive e l && boundedPositive e r) = (actualPositive l && actualPositive r)
      rw [ihl e hl, ihr e hr]

theorem boundedMay_exact_of_sufficient :
    ∀ (t : Term) (d : Nat), size t ≤ d → boundedMay d t = actualMay t := by
  intro t
  induction t with
  | atom p =>
    intro d hd
    cases d with
    | zero => exfalso; simp only [size] at hd; omega
    | succ e => rfl
  | node l r ihl ihr =>
    intro d hd
    cases d with
    | zero => exfalso; simp only [size] at hd; omega
    | succ e =>
      have hl : size l ≤ e := by simp only [size] at hd; omega
      have hr : size r ≤ e := by simp only [size] at hd; omega
      show (boundedMay e l || boundedMay e r) = (actualMay l || actualMay r)
      rw [ihl e hl, ihr e hr]

/-! ### Soundness under exhaustion (the theorem the mission asks for)

`boundedPositive d x = true → actual x = true`: a bounded positive relation
never claims a truth the exact relation does not have, budget-sufficient or
not — because at exhaustion it can only answer `false`, never `true`,
matching `TestSubtypeDepthExhaustionFailsClosed`'s
"depth exhaustion must still fail closed and report false rather than let
the walk run to completion." -/

theorem boundedPositive_sound :
    ∀ (t : Term) (d : Nat), boundedPositive d t = true → actualPositive t = true := by
  intro t
  induction t with
  | atom p =>
    intro d h
    cases d with
    | zero => simp [boundedPositive] at h
    | succ e => simpa [boundedPositive, actualPositive] using h
  | node l r ihl ihr =>
    intro d h
    cases d with
    | zero => simp [boundedPositive] at h
    | succ e =>
      have h' : boundedPositive e l = true ∧ boundedPositive e r = true := by
        simpa [boundedPositive, Bool.and_eq_true] using h
      have hl := ihl e h'.1
      have hr := ihr e h'.2
      simp [actualPositive, hl, hr]

/-- Dually: `boundedMay d x = false → ¬ actualMay x`, matching
`numericForMayContainNumber`'s doc comment — a pathological chain fails
closed (answers `true`, "can't rule it out") instead of falsely proving the
operand cannot satisfy the query. A bounded `false` is only ever produced by
fully exploring every branch (fuel never ran out along any of them), so it
faithfully reports the exact relation's `false`. -/
theorem boundedMay_sound :
    ∀ (t : Term) (d : Nat), boundedMay d t = false → actualMay t = false := by
  intro t
  induction t with
  | atom p =>
    intro d h
    cases d with
    | zero => simp [boundedMay] at h
    | succ e => simpa [boundedMay, actualMay] using h
  | node l r ihl ihr =>
    intro d h
    cases d with
    | zero => simp [boundedMay] at h
    | succ e =>
      have h' : boundedMay e l = false ∧ boundedMay e r = false := by
        simpa [boundedMay, Bool.or_eq_false_iff] using h
      have hl := ihl e h'.1
      have hr := ihr e h'.2
      simp [actualMay, hl, hr]

end GoLua.DepthPolarity
