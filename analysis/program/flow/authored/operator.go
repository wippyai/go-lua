package authored

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// Unary, Binary, and Select are authored scalar operations. Their owner is
// only lexical Body provenance; control, entries, and outcomes are finalizer
// work rather than content-addressed rows.
type Unary struct {
	Owner   keyspace.Term
	Op      kind.UnaryOp
	Operand keyspace.Term
}

type Binary struct {
	Owner       keyspace.Term
	Op          kind.BinaryOp
	Left, Right keyspace.Term
}

type Select struct {
	Owner       keyspace.Term
	Op          kind.SelectOp
	Left, Right keyspace.Term
}

func validUnaryOp(op kind.UnaryOp) bool { return op >= kind.UnaryNeg && op <= kind.UnaryBitNot }

func validBinaryOp(op kind.BinaryOp) bool {
	return op >= kind.BinaryAdd && op <= kind.BinaryGreaterEqual
}

func validSelectOp(op kind.SelectOp) bool { return op == kind.SelectAnd || op == kind.SelectOr }

func (view Operators) Unaries() Unaries {
	return Unaries(view)
}
func (view Operators) Binaries() Binaries {
	return Binaries(view)
}
func (view Operators) Selects() Selects {
	return Selects(view)
}

func (view Unaries) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.operators.unaries)
}

func (view Unaries) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyUnary, index, len(view.component.operators.unaries))
}

func (view Unaries) Get(term keyspace.Term) (owner keyspace.Term, op kind.UnaryOp, operand keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyUnary, len(view.component.operators.unaries)) {
		return 0, 0, 0, false
	}
	row := view.component.operators.unaries[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Op, row.Operand, true
}

func (view Binaries) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.operators.binaries)
}

func (view Binaries) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilyBinary, index, len(view.component.operators.binaries))
}

func (view Binaries) Get(term keyspace.Term) (owner keyspace.Term, op kind.BinaryOp, left, right keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilyBinary, len(view.component.operators.binaries)) {
		return 0, 0, 0, 0, false
	}
	row := view.component.operators.binaries[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Op, row.Left, row.Right, true
}

func (view Selects) Count() int {
	if !view.active() {
		return 0
	}
	return len(view.component.operators.selects)
}

func (view Selects) At(index int) (keyspace.Term, bool) {
	if !view.active() {
		return 0, false
	}
	return termAt(keyspace.FamilySelect, index, len(view.component.operators.selects))
}

func (view Selects) Get(term keyspace.Term) (owner keyspace.Term, op kind.SelectOp, left, right keyspace.Term, ok bool) {
	if !view.active() || !keyspace.ValidTerm(term, keyspace.FamilySelect, len(view.component.operators.selects)) {
		return 0, 0, 0, 0, false
	}
	row := view.component.operators.selects[keyspace.TermOrdinal(term)-1]
	return row.Owner, row.Op, row.Left, row.Right, true
}
