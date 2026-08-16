package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (c *Collector) Return(span source.Span, owner, values keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyReturn, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitReturn(c.counts, term, owner, values); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Break(span source.Span, owner keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyBreak, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitBreak(c.counts, term, owner); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Label(span source.Span, owner keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyLabel, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitLabel(c.counts, term, owner); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Goto(span source.Span, owner, target keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyGoto, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitGoto(c.counts, term, owner, target); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Branch(span source.Span, owner, condition, whenTrue, whenFalse keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyBranch, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitBranch(c.counts, term, owner, condition, whenTrue, whenFalse); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Loop(span source.Span, owner, body, control keyspace.Term, cells []keyspace.Term, loopKind kind.LoopKind) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyLoop, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitLoop(c.counts, term, owner, body, control, cells, loopKind); err != nil {
		c.fail(err)
		return 0
	}
	return term
}
