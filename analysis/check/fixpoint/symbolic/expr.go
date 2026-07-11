package symbolic

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

// ValueOp enumerates the symbolic value expression forms. A transformer's
// values are a DAG over these ops; evaluation happens only at instantiation,
// against a concrete caller binding.
type ValueOp uint8

const (
	// OpConst is a caller-independent product value computed by the body.
	OpConst ValueOp = iota
	// OpRead is the value reachable at a symbolic path at entry.
	OpRead
	// OpJoin is the lattice join of the argument expressions.
	OpJoin
	// OpMeet is the lattice meet of the argument expressions.
	OpMeet
	// OpNarrow applies the body's proven narrowing to the argument: at
	// instantiation the result is Narrow(arg, Const) with the same operator
	// the concrete solver applies.
	OpNarrow
	// OpCallResult is result slot Slot of an unresolved call node; resolved
	// during SCC composition or at instantiation.
	OpCallResult
	// OpAllocation is a fresh allocation root's value shape.
	OpAllocation
)

// ExprID indexes a ValueExpr inside one transformer's expression table.
// The zero value is invalid; valid ids are 1-based.
type ExprID uint32

// CallID indexes an unresolved call node inside one transformer.
type CallID uint32

// ValueExpr is one node of the symbolic value DAG. Exactly the fields
// selected by Op are meaningful.
type ValueExpr struct {
	Op    ValueOp
	Const product.Value // OpConst, OpNarrow (the narrowing operand)
	Path  Path          // OpRead
	Args  []ExprID      // OpJoin, OpMeet, OpNarrow (single arg)
	Call  CallID        // OpCallResult
	Slot  int           // OpCallResult result slot; OpAllocation ordinal
}

// Exprs is the interned expression table of one transformer. Structural
// dedup keeps the DAG canonical: building the same expression twice yields
// the same ExprID, so transformer equality can compare ids.
type Exprs struct {
	nodes []ValueExpr
	index map[exprKey]ExprID
	reg   *axis.Registry
}

type exprKey struct {
	op    ValueOp
	path  string
	args  string
	call  CallID
	slot  int
	cnst  uint64
	valid bool
}

// NewExprs creates an empty expression table bound to the value registry the
// product constants live under.
func NewExprs(reg *axis.Registry) *Exprs {
	return &Exprs{index: make(map[exprKey]ExprID), reg: reg}
}

// Len returns the number of interned expressions.
func (e *Exprs) Len() int { return len(e.nodes) }

// At returns the expression for id. The zero id returns a zero expression.
func (e *Exprs) At(id ExprID) ValueExpr {
	if id == 0 || int(id) > len(e.nodes) {
		return ValueExpr{}
	}
	return e.nodes[id-1]
}

func (e *Exprs) keyOf(x ValueExpr) exprKey {
	k := exprKey{op: x.Op, call: x.Call, slot: x.Slot, valid: true}
	switch x.Op {
	case OpConst, OpNarrow:
		k.cnst = product.Hash(e.reg, x.Const)
	}
	if x.Op == OpRead {
		k.path = x.Path.String()
	}
	if len(x.Args) > 0 {
		var b []byte
		for _, a := range x.Args {
			b = append(b, byte(a), byte(a>>8), byte(a>>16), byte(a>>24))
		}
		k.args = string(b)
	}
	return k
}

// Intern deduplicates x and returns its id. Join/Meet argument lists are
// normalized (sorted, deduplicated) so operand order cannot mint distinct
// spellings of the same expression.
func (e *Exprs) Intern(x ValueExpr) ExprID {
	if x.Op == OpJoin || x.Op == OpMeet {
		args := append([]ExprID(nil), x.Args...)
		sort.Slice(args, func(i, j int) bool { return args[i] < args[j] })
		dedup := args[:0]
		var prev ExprID
		for _, a := range args {
			if a != prev {
				dedup = append(dedup, a)
			}
			prev = a
		}
		if len(dedup) == 1 {
			return dedup[0]
		}
		x.Args = dedup
	}
	k := e.keyOf(x)
	if id, ok := e.index[k]; ok {
		// Hash collision on constants is resolved by content comparison.
		if x.Op != OpConst && x.Op != OpNarrow {
			return id
		}
		if product.Equal(e.reg, e.nodes[id-1].Const, x.Const) {
			return id
		}
	}
	e.nodes = append(e.nodes, x)
	id := ExprID(len(e.nodes))
	e.index[k] = id
	return id
}

// GuardOp enumerates guard predicate forms over symbolic paths. Guards
// condition transformer clauses; an unsatisfiable guard drops a clause only
// at instantiation, never during construction.
type GuardOp uint8

const (
	// GuardTruthy holds when the path's value is truthy at entry.
	GuardTruthy GuardOp = iota
	// GuardFalsy holds when the path's value is falsy at entry.
	GuardFalsy
	// GuardPresent holds when the path is present (non-nil) at entry.
	GuardPresent
	// GuardAbsent holds when the path is nil/absent at entry.
	GuardAbsent
)

// Guard is one predicate over a symbolic path.
type Guard struct {
	Op   GuardOp
	Path Path
}

// Less orders guards canonically.
func (g Guard) Less(o Guard) bool {
	if g.Op != o.Op {
		return g.Op < o.Op
	}
	return g.Path.Less(o.Path)
}

// Equal reports structural guard equality.
func (g Guard) Equal(o Guard) bool {
	return g.Op == o.Op && g.Path.Equal(o.Path)
}

// GuardSet is a canonical conjunction of guards: sorted, deduplicated.
// The empty set is the always-true guard.
type GuardSet []Guard

// NormalizeGuards returns the canonical form of gs.
func NormalizeGuards(gs []Guard) GuardSet {
	out := append(GuardSet(nil), gs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	dedup := out[:0]
	for i, g := range out {
		if i == 0 || !g.Equal(out[i-1]) {
			dedup = append(dedup, g)
		}
	}
	return dedup
}

// Equal reports canonical guard-set equality.
func (gs GuardSet) Equal(o GuardSet) bool {
	if len(gs) != len(o) {
		return false
	}
	for i := range gs {
		if !gs[i].Equal(o[i]) {
			return false
		}
	}
	return true
}

// Less orders guard sets canonically (shorter first, then lexicographic).
func (gs GuardSet) Less(o GuardSet) bool {
	if len(gs) != len(o) {
		return len(gs) < len(o)
	}
	for i := range gs {
		if !gs[i].Equal(o[i]) {
			return gs[i].Less(o[i])
		}
	}
	return false
}
