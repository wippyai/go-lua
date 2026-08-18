package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Cell creates one local lexical Cell and atomically attaches its authored
// spelling when supplied. The optional name is binder-owned metadata; no
// lowerer may derive it from parser syntax.
func (c *Collector) Cell(span source.Span, body keyspace.Term, name string) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyCell, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitCell(c.counts, term, body); err != nil {
		c.fail(err)
		return 0
	}
	if !c.source.SetCellSpelling(term, name) {
		c.fail(errors.New("program/lower/collector: could not attach Cell spelling"))
		return 0
	}
	return term
}

// Global selects a binder-censused Program-scoped Cell. Global identity is
// resolved only through the owning census; raw names and lower spans are not
// lookup authorities. The reserved Source span and Cell ordinal were fixed
// before traversal.
func (c *Collector) Global(identity bind.GlobalIdentity) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	if !identity.Valid() {
		c.fail(errors.New("program/lower/collector: invalid global identity"))
		return 0
	}
	cell, ok := c.flow.GlobalCensus().Cell(identity)
	if !ok {
		c.fail(errors.New("program/lower/collector: global identity is foreign or unreserved"))
		return 0
	}
	return keyspace.MakeTerm(keyspace.FamilyCell, cell.Ordinal())
}

// Read records one ordinary storage/lens observation.
func (c *Collector) Read(span source.Span, owner, subject keyspace.Term) keyspace.Term {
	return c.read(span, owner, subject, false)
}

// ImplicitRead records binder-proven implicitness on the ordinary Read row.
func (c *Collector) ImplicitRead(span source.Span, owner, global keyspace.Term) keyspace.Term {
	return c.read(span, owner, global, true)
}

func (c *Collector) read(span source.Span, owner, subject keyspace.Term, implicit bool) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyRead, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitRead(c.counts, term, owner, subject, implicit); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

func (c *Collector) Vararg(span source.Span, owner, cell keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyVararg, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitVararg(c.counts, term, owner, cell); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// Bind records the Flow Bind row and immediately appends Source-owned Cell
// order. There is no Flow-side order shadow to reconcile later.
func (c *Collector) Bind(span source.Span, owner keyspace.Term, cells []keyspace.Term, values keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	sourceCellsAlreadyOrdered := false
	for _, cell := range cells {
		if sourceCellAlreadyOrdered(c, cell) {
			sourceCellsAlreadyOrdered = true
			break
		}
	}
	term := c.mint(keyspace.FamilyBind, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitBind(c.counts, term, owner, values, cells, sourceCellsAlreadyOrdered); err != nil {
		c.fail(err)
		return 0
	}
	if !c.BindCells(term, cells) {
		return 0
	}
	return term
}

// Assign creates one Assign and one fresh ordered Write per target. Target
// spans are passed independently so a write never silently inherits the
// statement span.
func (c *Collector) Assign(span source.Span, owner keyspace.Term, targets []keyspace.Term, targetSpans []source.Span, values keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	assignTerm := c.mint(keyspace.FamilyAssign, span)
	if assignTerm == 0 {
		return 0
	}
	if err := c.flow.AdmitAssign(c.counts, assignTerm, owner, values, targets, len(targets) == len(targetSpans)); err != nil {
		c.fail(err)
		return 0
	}
	for index, target := range targets {
		writeTerm := c.mint(keyspace.FamilyWrite, targetSpans[index])
		if writeTerm == 0 {
			return 0
		}
		if err := c.flow.AdmitWrite(c.counts, writeTerm, assignTerm, target); err != nil {
			c.fail(err)
			return 0
		}
	}
	return assignTerm
}
