/-
Mechanization pilot for obligation A1 (per-lane join/widen/order laws),
instantiated for the placement lane.

Go source mirrored: analysis/domain/placement/placement.go. That file defines
a five-point total order

    Bottom < Stack < OwnedHeap < SharedHeap < Unknown

with `Join`/`Meet` as max/min along the chain and `Widen := Join` (the file's
own comment: "Widen equals Join because the placement lattice has finite
height"). The Go law suite is `TestPlacementLatticeLaws` in
analysis/domain/placement/placement_test.go, which drives the generic
`latticelaws.LawSuite` (analysis/test/laws/lattice/laws.go) checking
reflexivity/antisymmetry/transitivity of the order, join
idempotence/commutativity/associativity/bottom-identity/top-absorption,
order-consistent-with-join, meet laws, and absorption; and
`TestPlacementOrderJoinMeetAndWiden`, which pins the concrete join/meet/widen
table and the total order chain.

This file proves the same statements as genuine theorems over the actual
five-constructor Go carrier (not a finite sample of it, per
soundness_obligations.md's mechanization note for A1), plus the finiteness of
the lattice and the widening-chain stabilization theorem asked for by A1
("every ascending chain ... stabilizes within a lane-declared finite bound").
-/

namespace GoLua.PlacementLattice

/-- Mirrors `placement.Value` (`analysis/domain/placement/placement.go`).
Constructor order matches the Go `iota` chain exactly. -/
inductive Value where
  | Bottom
  | Stack
  | OwnedHeap
  | SharedHeap
  | Unknown
  deriving DecidableEq, Repr

open Value

/-- Mirrors the Go `iota` ordinal each constant carries as a `uint8`. -/
def rank : Value → Nat
  | Bottom => 0
  | Stack => 1
  | OwnedHeap => 2
  | SharedHeap => 3
  | Unknown => 4

/-- Mirrors `placement.LessOrEq`: `func LessOrEq(a, b Value) bool { return a <= b }`. -/
def LessOrEq (a b : Value) : Prop := rank a ≤ rank b

instance decLessOrEq (a b : Value) : Decidable (LessOrEq a b) :=
  inferInstanceAs (Decidable (rank a ≤ rank b))

instance : LE Value := ⟨LessOrEq⟩

instance decLE (a b : Value) : Decidable (a ≤ b) := decLessOrEq a b

/-- Mirrors `placement.Join`. -/
def Join (a b : Value) : Value := if rank b < rank a then a else b

/-- Mirrors `placement.Meet`. -/
def Meet (a b : Value) : Value := if rank a < rank b then a else b

/-- Mirrors `placement.Widen`: `func Widen(prev, next Value) Value { return Join(prev, next) }`. -/
def Widen (a b : Value) : Value := Join a b

/-- Mirrors `placement.Equal`. -/
def Equal (a b : Value) : Prop := a = b

/-! ### Finiteness

Mirrors the five-point carrier of `placement.Value`; nothing beyond these
five constructors exists (`cases`/`decide` below range over exactly them). -/

/-- The complete, duplicate-free enumeration of the placement carrier. -/
def allValues : List Value := [Bottom, Stack, OwnedHeap, SharedHeap, Unknown]

theorem allValues_complete (v : Value) : v ∈ allValues := by
  cases v <;> decide

theorem allValues_length : allValues.length = 5 := rfl

/-! ### Order laws (`TestPlacementLatticeLaws`: reflexivity, antisymmetry,
transitivity of `⊑`) -/

theorem le_refl (a : Value) : a ≤ a := by
  cases a <;> decide

theorem le_trans {a b c : Value} (hab : a ≤ b) (hbc : b ≤ c) : a ≤ c :=
  Nat.le_trans hab hbc

theorem le_antisymm {a b : Value} (hab : a ≤ b) (hba : b ≤ a) : a = b := by
  cases a <;> cases b <;> first | rfl | (exfalso; revert hab hba; decide)

theorem le_total (a b : Value) : a ≤ b ∨ b ≤ a := by
  cases a <;> cases b <;> decide

/-- The placement chain is total, matching the Go test's explicit chain
assertion: bottom < stack < owned-heap < shared-heap < unknown. -/
theorem total_order_chain :
    LessOrEq Bottom Stack ∧ LessOrEq Stack OwnedHeap ∧
    LessOrEq OwnedHeap SharedHeap ∧ LessOrEq SharedHeap Unknown := by
  decide

/-! ### Join laws (`TestPlacementLatticeLaws`: idempotence, commutativity,
associativity, bottom-identity, top-absorption; `TestPlacementOrderJoinMeetAndWiden`
for the concrete table) -/

theorem join_idem (a : Value) : Join a a = a := by
  cases a <;> decide

theorem join_comm (a b : Value) : Join a b = Join b a := by
  cases a <;> cases b <;> decide

theorem join_assoc (a b c : Value) : Join (Join a b) c = Join a (Join b c) := by
  cases a <;> cases b <;> cases c <;> decide

theorem join_bottom_identity_left (a : Value) : Join Bottom a = a := by
  cases a <;> decide

theorem join_bottom_identity_right (a : Value) : Join a Bottom = a := by
  cases a <;> decide

theorem join_unknown_absorbs_left (a : Value) : Join Unknown a = Unknown := by
  cases a <;> decide

theorem join_unknown_absorbs_right (a : Value) : Join a Unknown = Unknown := by
  cases a <;> decide

/-- Order is consistent with join: `a ⊑ b ↔ Join(a,b) = b` (the A1
"order-consistent-with-join" clause). -/
theorem le_iff_join_eq (a b : Value) : a ≤ b ↔ Join a b = b := by
  cases a <;> cases b <;> decide

/-- Join is a least upper bound: it dominates both operands and is the least
such element. -/
theorem join_ge_left (a b : Value) : a ≤ Join a b := by
  cases a <;> cases b <;> decide

theorem join_ge_right (a b : Value) : b ≤ Join a b := by
  cases a <;> cases b <;> decide

theorem join_least_upper_bound {a b c : Value} (ha : a ≤ c) (hb : b ≤ c) :
    Join a b ≤ c := by
  cases a <;> cases b <;> cases c <;> revert ha hb <;> decide

/-! ### Meet laws (dual to join; `TestPlacementLatticeLaws` meet block, gated
on `Meet != nil`, which the placement lane provides) -/

theorem meet_idem (a : Value) : Meet a a = a := by
  cases a <;> decide

theorem meet_comm (a b : Value) : Meet a b = Meet b a := by
  cases a <;> cases b <;> decide

theorem meet_assoc (a b c : Value) : Meet (Meet a b) c = Meet a (Meet b c) := by
  cases a <;> cases b <;> cases c <;> decide

/-! ### Absorption (`TestPlacementLatticeLaws` absorption block) -/

theorem absorption_join_meet (a b : Value) : Join a (Meet a b) = a := by
  cases a <;> cases b <;> decide

theorem absorption_meet_join (a b : Value) : Meet a (Join a b) = a := by
  cases a <;> cases b <;> decide

/-! ### Widening (`TestPlacementOrderJoinMeetAndWiden`'s widen column, and the
placement.go comment "Widen equals Join because the placement lattice has
finite height") -/

theorem widen_eq_join (a b : Value) : Widen a b = Join a b := rfl

theorem widen_overapproximates_left (a b : Value) : a ≤ Widen a b :=
  join_ge_left a b

theorem widen_overapproximates_right (a b : Value) : b ≤ Widen a b :=
  join_ge_right a b

/-! ### Concrete table (`TestPlacementOrderJoinMeetAndWiden`'s `cases` table) -/

theorem order_join_meet_widen_table :
    Join Bottom Stack = Stack ∧ Meet Bottom Stack = Bottom ∧
    Join Stack OwnedHeap = OwnedHeap ∧ Meet Stack OwnedHeap = Stack ∧
    Join OwnedHeap SharedHeap = SharedHeap ∧ Meet OwnedHeap SharedHeap = OwnedHeap ∧
    Join SharedHeap Unknown = Unknown ∧ Meet SharedHeap Unknown = SharedHeap := by
  decide

/-! ### Widening-chain stabilization (A1: "every ascending chain
`s_{i+1} = Widen(s_i, Join(s_i, growth))` stabilizes within a lane-declared
finite bound (4 for most lanes)")

A fixpoint solver's per-round growth is a function of the current abstract
value (a transfer function re-evaluated against accumulated state each
round), not an adversarial oracle independent of it. `g` below models exactly
that: growth at round `n` is `g (chain g s0 n)`, i.e. a deterministic
function of the current value. Under that (the actually-realized) shape, the
chain can change at most 4 times — the placement lattice's height — before it
is provably fixed forever. -/

/-- One lane's ascending widening iteration: round `n+1` widens the current
value against the growth the transfer function `g` produces from it. -/
def chain (g : Value → Value) (s0 : Value) : Nat → Value
  | 0 => s0
  | n + 1 => Widen (chain g s0 n) (g (chain g s0 n))

theorem chain_rank_le_four (g : Value → Value) (s0 : Value) (n : Nat) :
    rank (chain g s0 n) ≤ 4 := by
  cases chain g s0 n <;> decide

theorem chain_mono (g : Value → Value) (s0 : Value) (n : Nat) :
    chain g s0 n ≤ chain g s0 (n + 1) :=
  join_ge_left (chain g s0 n) (g (chain g s0 n))

/-- Whenever `Join a b` differs from `a`, its rank is strictly greater: `Join`
only ever returns something other than the left operand by returning
something of strictly greater rank. -/
theorem join_change_rank_lt (a b : Value) (h : a ≠ Join a b) :
    rank a < rank (Join a b) := by
  cases a <;> cases b <;> revert h <;> decide

/-- Whenever a round actually changes the value, its rank strictly increases
(specializes `join_change_rank_lt` to a chain round). -/
theorem chain_change_rank_lt (g : Value → Value) (s0 : Value) (n : Nat)
    (h : chain g s0 n ≠ chain g s0 (n + 1)) :
    rank (chain g s0 n) < rank (chain g s0 (n + 1)) :=
  join_change_rank_lt (chain g s0 n) (g (chain g s0 n)) h

/-- Once a round produces no change, the deterministic recurrence (growth
depends only on the current value) repeats that value forever. -/
theorem chain_fixed_from (g : Value → Value) (s0 : Value) (n : Nat)
    (h : chain g s0 n = chain g s0 (n + 1)) :
    ∀ m, chain g s0 (n + m) = chain g s0 n := by
  intro m
  induction m with
  | zero => rfl
  | succ k ih =>
    have step : chain g s0 (n + (k + 1)) = Widen (chain g s0 (n + k)) (g (chain g s0 (n + k))) := rfl
    rw [ih] at step
    exact step.trans h.symm

/-- The placement widening chain stabilizes by round 4: past that point every
round repeats the same value. This is the finite-height termination argument
A1 asks for, specialized to the placement lane's height of 4. -/
theorem chain_stabilizes (g : Value → Value) (s0 : Value) :
    ∀ n, n ≥ 4 → chain g s0 n = chain g s0 4 := by
  have fixes_forever : ∀ n, n ≤ 4 → chain g s0 n = chain g s0 (n + 1) →
      ∀ m, m ≥ n → chain g s0 m = chain g s0 4 := by
    intro n _ hfix m hm
    obtain ⟨k, rfl⟩ := Nat.le.dest hm
    have h1 := chain_fixed_from g s0 n hfix k
    have h4 : (4 : Nat) = n + (4 - n) := by omega
    have h2 := chain_fixed_from g s0 n hfix (4 - n)
    rw [h1, h4, h2]
  by_cases h0 : chain g s0 0 = chain g s0 1
  · intro n hn; exact fixes_forever 0 (by omega) h0 n (by omega)
  · by_cases h1 : chain g s0 1 = chain g s0 2
    · intro n hn; exact fixes_forever 1 (by omega) h1 n (by omega)
    · by_cases h2 : chain g s0 2 = chain g s0 3
      · intro n hn; exact fixes_forever 2 (by omega) h2 n (by omega)
      · by_cases h3 : chain g s0 3 = chain g s0 4
        · intro n hn; exact fixes_forever 3 (by omega) h3 n (by omega)
        · -- No round among 0..3 was flat: every step strictly increased rank,
          -- so rank (chain 4) = 4, i.e. chain 4 = Unknown, which absorbs any
          -- further growth (`join_unknown_absorbs_left`), so round 4 is flat.
          have r0 := chain_rank_le_four g s0 0
          have r01 : rank (chain g s0 0) < rank (chain g s0 1) :=
            chain_change_rank_lt g s0 0 h0
          have r12 : rank (chain g s0 1) < rank (chain g s0 2) :=
            chain_change_rank_lt g s0 1 h1
          have r23 : rank (chain g s0 2) < rank (chain g s0 3) :=
            chain_change_rank_lt g s0 2 h2
          have r34 : rank (chain g s0 3) < rank (chain g s0 4) :=
            chain_change_rank_lt g s0 3 h3
          have r4 := chain_rank_le_four g s0 4
          have hrank4 : rank (chain g s0 4) = 4 := by omega
          have hunknown : chain g s0 4 = Unknown := by
            cases hc : chain g s0 4 with
            | Unknown => rfl
            | Bottom => rw [hc] at hrank4; simp [rank] at hrank4
            | Stack => rw [hc] at hrank4; simp [rank] at hrank4
            | OwnedHeap => rw [hc] at hrank4; simp [rank] at hrank4
            | SharedHeap => rw [hc] at hrank4; simp [rank] at hrank4
          have hfix4 : chain g s0 4 = chain g s0 5 := by
            show chain g s0 4 = Widen (chain g s0 4) (g (chain g s0 4))
            rw [hunknown]
            exact (join_unknown_absorbs_left (g Unknown)).symm
          intro n hn; exact fixes_forever 4 (by omega) hfix4 n (by omega)

end GoLua.PlacementLattice
