package collector

import (
	"errors"

	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// Body declares an empty authored Body. SetBody must fill it exactly once.
func (o SourceOrder) Body(span source.Span) Term {
	c := o.collector
	term := c.mint(keyspace.FamilyBody, span)
	if term == 0 {
		return 0
	}
	c.source.bodies = append(c.source.bodies, source.BodySource{Body: term})
	c.source.filled = append(c.source.filled, false)
	return term
}

// SetBody installs one Body's exact direct source order. It never derives
// statement roots or containment; those belong to typed Flow finalization.
func (o SourceOrder) SetBody(body Term, terms ...Term) bool {
	c := o.collector
	if !mutationReady(c) {
		return false
	}
	if !validBody(c, body) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Body fill"))
	}
	at := int(keyspace.TermOrdinal(body) - 1)
	if at < 0 || at >= len(c.source.bodies) || c.source.filled[at] {
		c.fail(errors.New("program/lower/collector: Body filled more than once or out of order"))
		return false
	}
	copyTerms := append([]Term(nil), terms...)
	seen := make(map[Term]struct{}, len(copyTerms))
	for _, term := range copyTerms {
		if !validDirectBodyTerm(c, body, term) {
			return rejectMutation(c, errors.New("program/lower/collector: Body contains invalid authored Term"))
		}
		if _, duplicate := seen[term]; duplicate || sourceBodyTermSeen(c, term) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate direct Body root"))
		}
		seen[term] = struct{}{}
	}
	c.source.bodies[at].Terms = copyTerms
	c.source.filled[at] = true
	return true
}

func sourceBodyTermSeen(c *Collector, term Term) bool {
	if c == nil {
		return false
	}
	for index, row := range c.source.bodies {
		if index >= len(c.source.filled) || !c.source.filled[index] {
			continue
		}
		for _, existing := range row.Terms {
			if existing == term {
				return true
			}
		}
	}
	return false
}

// SetEntry fixes the one top-level Body. It is a scalar construction fact;
// Source validates the completed forest when Flow consumes its Finalizer.
func (o SourceOrder) SetEntry(body Term) bool {
	c := o.collector
	if !mutationReady(c) {
		return false
	}
	if !validBody(c, body) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Entry Body"))
	}
	if c.source.entry != 0 {
		return rejectMutation(c, errors.New("program/lower/collector: Entry already assigned"))
	}
	c.source.entry = body
	return true
}

// Entry returns the scalar entry Body while construction remains local. It
// does not expose Source or a query authority.
func (o SourceOrder) Entry() Term {
	c := o.collector
	if c == nil || c.err != nil {
		return 0
	}
	return c.source.entry
}

// BindCells records Source's authored Cell order for one Bind. The evaluated
// Values relation remains Flow-owned; this helper stores only the ordered Cell
// provenance required by Source.Build.
func (o SourceOrder) BindCells(bind Term, cells []Term) bool {
	c := o.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, bind, keyspace.FamilyBind) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Bind order owner"))
	}
	at := int(keyspace.TermOrdinal(bind) - 1)
	if at != len(c.source.binds) {
		return rejectMutation(c, errors.New("program/lower/collector: Bind order is not dense"))
	}
	if at < 0 || at >= len(c.flow.storage.binds) ||
		!validFamilyTerm(c, c.flow.storage.binds[at].Owner, keyspace.FamilyBody) {
		return rejectMutation(c, errors.New("program/lower/collector: Bind order owner is absent or foreign"))
	}
	owner := c.flow.storage.binds[at].Owner
	seen := make(map[Term]struct{}, len(cells))
	for _, cell := range cells {
		if !localCellInBodyAdmission(c, cell, owner) {
			return rejectMutation(c, errors.New("program/lower/collector: invalid Bind Cell"))
		}
		if _, duplicate := seen[cell]; duplicate || sourceCellAlreadyOrdered(c, cell) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate Bind Cell"))
		}
		seen[cell] = struct{}{}
	}
	c.source.binds = append(c.source.binds, source.BindCells{Bind: bind, Cells: append([]Term(nil), cells...)})
	return true
}

// FunctionFormals records Source's authored formal Cell order. Static
// signature relations are a separate owner and are not inferred here.
func (o SourceOrder) FunctionFormals(function Term, formals []Term) bool {
	c := o.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Function formal owner"))
	}
	at := int(keyspace.TermOrdinal(function) - 1)
	if at != len(c.source.functions) {
		return rejectMutation(c, errors.New("program/lower/collector: Function formal order is not dense"))
	}
	if at < 0 || at >= len(c.flow.functions.functions) || c.flow.functions.functions[at].Body == 0 ||
		!validFamilyTerm(c, c.flow.functions.functions[at].Body, keyspace.FamilyBody) {
		return rejectMutation(c, errors.New("program/lower/collector: Function formal owner body is absent or foreign"))
	}
	body := c.flow.functions.functions[at].Body
	seen := make(map[Term]struct{}, len(formals))
	for _, formal := range formals {
		if !localCellInBodyAdmission(c, formal, body) {
			return rejectMutation(c, errors.New("program/lower/collector: invalid Function formal Cell"))
		}
		if _, duplicate := seen[formal]; duplicate || sourceCellAlreadyOrdered(c, formal) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate Function formal Cell"))
		}
		seen[formal] = struct{}{}
	}
	c.source.functions = append(c.source.functions, source.FunctionFormals{Function: function, Formals: append([]Term(nil), formals...)})
	return true
}

func sourceCellAlreadyOrdered(c *Collector, cell Term) bool {
	if c == nil {
		return false
	}
	for _, row := range c.source.binds {
		for _, existing := range row.Cells {
			if existing == cell {
				return true
			}
		}
	}
	for _, row := range c.source.functions {
		for _, existing := range row.Formals {
			if existing == cell {
				return true
			}
		}
	}
	return false
}
