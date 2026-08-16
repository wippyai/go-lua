package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (c *Collector) Unary(span source.Span, owner keyspace.Term, op kind.UnaryOp, operand keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyUnary, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitUnary(c.counts, term, owner, op, operand); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Binary(span source.Span, owner keyspace.Term, op kind.BinaryOp, left, right keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyBinary, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitBinary(c.counts, term, owner, op, left, right); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Select(span source.Span, owner keyspace.Term, op kind.SelectOp, left, right keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilySelect, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitSelect(c.counts, term, owner, op, left, right); err != nil {
		c.fail(err)
		return 0
	}
	return term
}
