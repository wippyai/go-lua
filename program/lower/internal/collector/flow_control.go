package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (w FlowControlWriter) Return(span source.Span, owner, values keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !validFamilyTerm(c, values, keyspace.FamilyValues) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Return admission")
	}
	term := c.mint(keyspace.FamilyReturn, span)
	if term == 0 {
		return 0
	}
	c.flow.control.returns = append(c.flow.control.returns, flow.Return{Owner: owner, Values: values})
	return term
}

func (w FlowControlWriter) Break(span source.Span, owner keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Break admission")
	}
	term := c.mint(keyspace.FamilyBreak, span)
	if term == 0 {
		return 0
	}
	c.flow.control.breaks = append(c.flow.control.breaks, flow.Break{Owner: owner})
	return term
}

func (w FlowControlWriter) Label(span source.Span, owner keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Label admission")
	}
	term := c.mint(keyspace.FamilyLabel, span)
	if term == 0 {
		return 0
	}
	c.flow.control.labels = append(c.flow.control.labels, flow.Label{Owner: owner})
	return term
}

func (w FlowControlWriter) Goto(span source.Span, owner, target keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !validFamilyTerm(c, target, keyspace.FamilyLabel) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Goto admission")
	}
	term := c.mint(keyspace.FamilyGoto, span)
	if term == 0 {
		return 0
	}
	c.flow.control.gotos = append(c.flow.control.gotos, flow.Goto{Owner: owner, Target: target})
	return term
}

func (w FlowControlWriter) Branch(span source.Span, owner, condition, whenTrue, whenFalse keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !flowrole.ValueOccurrence(c.counts, condition) || !validFamilyTerm(c, whenTrue, keyspace.FamilyBody) || !validFamilyTerm(c, whenFalse, keyspace.FamilyBody) || owner == whenTrue || owner == whenFalse || whenTrue == whenFalse {
		return rejectTermMutationf(c, "program/lower/collector: invalid Branch admission")
	}
	term := c.mint(keyspace.FamilyBranch, span)
	if term == 0 {
		return 0
	}
	c.flow.control.branches = append(c.flow.control.branches, flow.Branch{Owner: owner, Condition: condition, WhenTrue: whenTrue, WhenFalse: whenFalse})
	return term
}

func (w FlowControlWriter) Loop(span source.Span, owner, body, control keyspace.Term, cells []keyspace.Term, loopKind kind.LoopKind) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) || !validFamilyTerm(c, body, keyspace.FamilyBody) || owner == body || loopKind < kind.LoopWhile || loopKind > kind.LoopGenericFor || !flowrole.LoopControlFamily(c.counts, control, loopKind) || !loopControlShapeAdmission(c, control, loopKind) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Loop admission")
	}
	for index, cell := range cells {
		if !localCellInBodyAdmission(c, cell, body) {
			return rejectTermMutationf(c, "program/lower/collector: invalid Loop Cell")
		}
		for _, previous := range cells[:index] {
			if previous == cell {
				return rejectTermMutationf(c, "program/lower/collector: duplicate Loop Cell")
			}
		}
	}
	r, ok := rangeFor(len(c.flow.control.loopCells), len(cells))
	if !ok {
		return rejectTermMutationf(c, "program/lower/collector: Loop Cell range overflow")
	}
	term := c.mint(keyspace.FamilyLoop, span)
	if term == 0 {
		return 0
	}
	c.flow.control.loopCells = append(c.flow.control.loopCells, cells...)
	c.flow.control.loops = append(c.flow.control.loops, flow.Loop{Owner: owner, Body: body, Kind: loopKind, Control: control, Cells: r})
	return term
}

func loopControlShapeAdmission(c *Collector, control keyspace.Term, loopKind kind.LoopKind) bool {
	values, ok := valuesRowAdmission(c, control)
	if loopKind == kind.LoopWhile || loopKind == kind.LoopRepeat {
		return true
	}
	if !ok {
		return false
	}
	width := values.Fixed.End - values.Fixed.Start
	switch loopKind {
	case kind.LoopNumericFor:
		return (width == 2 || width == 3) && values.Tail == 0
	case kind.LoopGenericFor:
		return values.Fixed.Start != values.Fixed.End || values.Tail != 0
	default:
		return false
	}
}
