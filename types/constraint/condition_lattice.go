package constraint

// ConditionLattice is the abstract domain for path conditions.
//
// The carrier is DNF over the unbounded set of Constraint literals appearing
// syntactically in the program. The ordering is logical implication: a ⊑ b
// iff every state satisfying a also satisfies b — that is, b is at least as
// permissive as a. Bottom = FalseCondition (unsatisfiable), Top = TrueCondition
// (always satisfied).
//
// Today this lattice does NOT satisfy the ascending-chain condition under
// repeated meet over an unbounded literal set, and Widen is defined as Join
// (i.e. no widening). The lattice law harness in types/lattice catches both
// gaps as test failures, motivating Phase F (Forge journal seq 304).
type ConditionLattice struct{}

func (ConditionLattice) Bottom() Condition { return FalseCondition() }
func (ConditionLattice) Top() Condition    { return TrueCondition() }

func (ConditionLattice) Equal(a, b Condition) bool { return a.Equals(b) }

// LessOrEq encodes the implication ordering: a ⊑ b iff a implies b.
// Subsumes(x, y) is "x subsumes y" = "any state satisfying y also satisfies x"
// = "y implies x". So a ⊑ b corresponds to b.Subsumes(a).
func (ConditionLattice) LessOrEq(a, b Condition) bool { return b.Subsumes(a) }

func (ConditionLattice) Join(a, b Condition) Condition { return Or(a, b) }
func (ConditionLattice) Meet(a, b Condition) Condition { return And(a, b) }

// Widen is currently aliased to Join. The Condition domain over an unbounded
// literal set lacks the ascending-chain property, so this widening is unsound
// for termination: the law harness's chain-termination check will fail on
// pathological inputs. Phase F (Forge journal seq 304) replaces this with a
// real Cousot widening (reduced DNF + drop-unstable-disjuncts) or with a
// canonicalized representation (BDD / predicate abstraction).
func (l ConditionLattice) Widen(prev, next Condition) Condition {
	return l.Join(prev, next)
}
