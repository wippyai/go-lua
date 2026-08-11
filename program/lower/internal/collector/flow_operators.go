package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (w FlowOperatorsWriter) Unary(span source.Span, owner keyspace.Term, op kind.UnaryOp, operand keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || op < kind.UnaryNeg || op > kind.UnaryBitNot || !flowrole.ValueOccurrence(c.counts, operand) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Unary admission")
	}
	term := c.mint(keyspace.FamilyUnary, span)
	if term == 0 {
		return 0
	}
	c.flow.operators.unaries = append(c.flow.operators.unaries, flow.Unary{Owner: owner, Op: op, Operand: operand})
	return term
}

func (w FlowOperatorsWriter) Binary(span source.Span, owner keyspace.Term, op kind.BinaryOp, left, right keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || op < kind.BinaryAdd || op > kind.BinaryGreaterEqual || !flowrole.ValueOccurrence(c.counts, left) || !flowrole.ValueOccurrence(c.counts, right) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Binary admission")
	}
	term := c.mint(keyspace.FamilyBinary, span)
	if term == 0 {
		return 0
	}
	c.flow.operators.binaries = append(c.flow.operators.binaries, flow.Binary{Owner: owner, Op: op, Left: left, Right: right})
	return term
}

func (w FlowOperatorsWriter) Select(span source.Span, owner keyspace.Term, op kind.SelectOp, left, right keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || (op != kind.SelectAnd && op != kind.SelectOr) || !flowrole.ValueOccurrence(c.counts, left) || !flowrole.ValueOccurrence(c.counts, right) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Select admission")
	}
	term := c.mint(keyspace.FamilySelect, span)
	if term == 0 {
		return 0
	}
	c.flow.operators.selects = append(c.flow.operators.selects, flow.Select{Owner: owner, Op: op, Left: left, Right: right})
	return term
}
