package collector

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/program/flow"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Cell creates one local lexical Cell.
func (w FlowStorageWriter) Cell(span source.Span, body keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, body, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Cell body")
	}
	term := c.mint(keyspace.FamilyCell, span)
	if term == 0 {
		return 0
	}
	c.flow.storage.cells = append(c.flow.storage.cells, flow.Cell{Kind: flow.CellLocal, Body: body})
	return term
}

// Global selects a binder-censused Program-scoped Cell. Global identity is
// resolved only through the owning census; raw names and lower spans are not
// lookup authorities. The reserved Source span and Cell ordinal were fixed
// before traversal.
func (w FlowStorageWriter) Global(identity bind.GlobalIdentity) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !identity.Valid() {
		c.fail(errors.New("program/lower/collector: invalid global identity"))
		return 0
	}
	cell, ok := c.flow.storage.globalCensus.Cell(identity)
	if !ok {
		c.fail(errors.New("program/lower/collector: global identity is foreign or unreserved"))
		return 0
	}
	return keyspace.MakeTerm(keyspace.FamilyCell, cell.Ordinal())
}

// Read records one ordinary storage/lens observation.
func (w FlowStorageWriter) Read(span source.Span, owner, subject keyspace.Term) keyspace.Term {
	return w.read(span, owner, subject, false)
}

// ImplicitRead records binder-proven implicitness on the ordinary Read row.
func (w FlowStorageWriter) ImplicitRead(span source.Span, owner, global keyspace.Term) keyspace.Term {
	return w.read(span, owner, global, true)
}

func (w FlowStorageWriter) read(span source.Span, owner, subject keyspace.Term, implicit bool) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !flowrole.Addressable(c.counts, subject) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Read admission")
	}
	if implicit && !globalCellAdmission(c, subject) {
		return rejectTermMutationf(c, "program/lower/collector: invalid implicit Read global")
	}
	term := c.mint(keyspace.FamilyRead, span)
	if term == 0 {
		return 0
	}
	c.flow.storage.reads = append(c.flow.storage.reads, flow.Read{Owner: owner, Source: subject, Implicit: implicit})
	return term
}

func (w FlowStorageWriter) Vararg(span source.Span, owner, cell keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !localCellAdmission(c, cell) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Vararg admission")
	}
	term := c.mint(keyspace.FamilyVararg, span)
	if term == 0 {
		return 0
	}
	c.flow.storage.varargs = append(c.flow.storage.varargs, flow.Vararg{Owner: owner, Cell: cell})
	return term
}

// Bind records the Flow Bind row and immediately appends Source-owned Cell
// order. There is no Flow-side order shadow to reconcile later.
func (w FlowStorageWriter) Bind(span source.Span, owner keyspace.Term, cells []keyspace.Term, values keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !validFamilyTerm(c, values, keyspace.FamilyValues) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Bind admission")
	}
	seenCells := make(map[keyspace.Term]struct{}, len(cells))
	for _, cell := range cells {
		if !localCellInBodyAdmission(c, cell, owner) {
			return rejectTermMutationf(c, "program/lower/collector: invalid Bind Cell")
		}
		if _, duplicate := seenCells[cell]; duplicate || sourceCellAlreadyOrdered(c, cell) {
			return rejectTermMutationf(c, "program/lower/collector: duplicate Bind Cell")
		}
		seenCells[cell] = struct{}{}
	}
	_, ok := rangeFor(0, len(cells))
	if !ok {
		return rejectTermMutationf(c, "program/lower/collector: Bind Cell range overflow")
	}
	term := c.mint(keyspace.FamilyBind, span)
	if term == 0 {
		return 0
	}
	c.flow.storage.binds = append(c.flow.storage.binds, flow.Bind{Owner: owner, Values: values})
	if !c.Source().Order().BindCells(term, cells) {
		return 0
	}
	return term
}

// Assign creates one Assign and one fresh ordered Write per target. Target
// spans are passed independently so a write never silently inherits the
// statement span.
func (w FlowStorageWriter) Assign(span source.Span, owner keyspace.Term, targets []keyspace.Term, targetSpans []source.Span, values keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !validFamilyTerm(c, values, keyspace.FamilyValues) || len(targets) != len(targetSpans) || len(targets) == 0 {
		return rejectTermMutationf(c, "program/lower/collector: invalid Assign admission")
	}
	for _, target := range targets {
		if !flowrole.Addressable(c.counts, target) {
			return rejectTermMutationf(c, "program/lower/collector: invalid Assign target")
		}
	}
	assignTerm := c.mint(keyspace.FamilyAssign, span)
	if assignTerm == 0 {
		return 0
	}
	c.flow.storage.assigns = append(c.flow.storage.assigns, flow.Assign{Owner: owner, Values: values})
	for index, target := range targets {
		if c.mint(keyspace.FamilyWrite, targetSpans[index]) == 0 {
			return 0
		}
		c.flow.storage.writes = append(c.flow.storage.writes, flow.Write{Assign: assignTerm, Target: target})
	}
	return assignTerm
}

func (w FlowStorageWriter) Write(span source.Span, assign, target keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, assign, keyspace.FamilyAssign) || !flowrole.Addressable(c.counts, target) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Write admission")
	}
	term := c.mint(keyspace.FamilyWrite, span)
	if term == 0 {
		return 0
	}
	c.flow.storage.writes = append(c.flow.storage.writes, flow.Write{Assign: assign, Target: target})
	return term
}
