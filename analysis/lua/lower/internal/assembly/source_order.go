package assembly

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// Body declares an empty authored Body. SetBody must fill it exactly once.
func (c *Collector) Body(span source.Span) keyspace.Term {
	term := c.mint(keyspace.FamilyBody, span)
	if term == 0 {
		return 0
	}
	c.source.Bodies = append(c.source.Bodies, source.BodySource{Body: term})
	c.bodies = append(c.bodies, false)
	return term
}

// SetBody installs one Body's exact direct source order. It never derives
// statement roots or containment; those belong to typed Flow finalization.
func (c *Collector) SetBody(body keyspace.Term, terms ...keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validBody(c, body) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Body fill"))
	}
	copyTerms := append([]keyspace.Term(nil), terms...)
	seen := make(map[keyspace.Term]struct{}, len(copyTerms))
	for _, term := range copyTerms {
		if !validDirectBodyTerm(c, body, term) {
			return rejectMutation(c, errors.New("program/lower/collector: Body contains invalid authored Term"))
		}
		if _, duplicate := seen[term]; duplicate || sourceBodyTermSeen(c, term) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate direct Body root"))
		}
		seen[term] = struct{}{}
	}
	at := int(keyspace.TermOrdinal(body) - 1)
	if at < 0 || at >= len(c.source.Bodies) || at >= len(c.bodies) || c.bodies[at] {
		c.fail(errors.New("program/lower/collector: Body filled more than once or out of order"))
		return false
	}
	c.source.Bodies[at].Terms = copyTerms
	c.bodies[at] = true
	return true
}

func sourceBodyTermSeen(c *Collector, term keyspace.Term) bool {
	if c == nil {
		return false
	}
	for index, row := range c.source.Bodies {
		if index >= len(c.bodies) || !c.bodies[index] {
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
func (c *Collector) SetEntry(body keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validBody(c, body) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Entry Body"))
	}
	if c.entry != 0 {
		return rejectMutation(c, errors.New("program/lower/collector: Entry already assigned"))
	}
	c.entry = body
	return true
}

// Entry returns the scalar entry Body while construction remains local. It
// does not expose Source or a query authority.
func (c *Collector) Entry() keyspace.Term {
	if c == nil || c.err != nil {
		return 0
	}
	return c.entry
}

// BindCells records Source's authored Cell order for one Bind. The evaluated
// Values relation remains Flow-owned; this helper stores only the ordered Cell
// provenance required by Source.Build.
func (c *Collector) BindCells(bind keyspace.Term, cells []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, bind, keyspace.FamilyBind) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Bind order owner"))
	}
	at := int(keyspace.TermOrdinal(bind) - 1)
	if at != len(c.source.Binds) {
		return rejectMutation(c, errors.New("program/lower/collector: Bind order is not dense"))
	}
	bindRow, ok := c.flow.BindAt(at)
	if at < 0 || !ok || !validFamilyTerm(c, bindRow.Owner, keyspace.FamilyBody) {
		return rejectMutation(c, errors.New("program/lower/collector: Bind order owner is absent or foreign"))
	}
	owner := bindRow.Owner
	seen := make(map[keyspace.Term]struct{}, len(cells))
	for _, cell := range cells {
		if !localCellInBodyAdmission(c, cell, owner) {
			return rejectMutation(c, errors.New("program/lower/collector: invalid Bind Cell"))
		}
		if _, duplicate := seen[cell]; duplicate || sourceCellAlreadyOrdered(c, cell) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate Bind Cell"))
		}
		seen[cell] = struct{}{}
	}
	c.source.Binds = append(c.source.Binds, source.BindCells{Bind: bind, Cells: append([]keyspace.Term(nil), cells...)})
	return true
}

// FunctionFormals records Source's authored formal Cell order. Static
// signature relations are a separate owner and are not inferred here.
func (c *Collector) FunctionFormals(function keyspace.Term, formals []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, function, keyspace.FamilyFunction) {
		return rejectMutation(c, errors.New("program/lower/collector: invalid Function formal owner"))
	}
	at := int(keyspace.TermOrdinal(function) - 1)
	if at != len(c.source.Functions) {
		return rejectMutation(c, errors.New("program/lower/collector: Function formal order is not dense"))
	}
	functionRow, ok := c.flow.FunctionAt(at)
	if at < 0 || !ok || functionRow.Body == 0 || !validFamilyTerm(c, functionRow.Body, keyspace.FamilyBody) {
		return rejectMutation(c, errors.New("program/lower/collector: Function formal owner body is absent or foreign"))
	}
	body := functionRow.Body
	seen := make(map[keyspace.Term]struct{}, len(formals))
	for _, formal := range formals {
		if !localCellInBodyAdmission(c, formal, body) {
			return rejectMutation(c, errors.New("program/lower/collector: invalid Function formal Cell"))
		}
		if _, duplicate := seen[formal]; duplicate || sourceCellAlreadyOrdered(c, formal) {
			return rejectMutation(c, errors.New("program/lower/collector: duplicate Function formal Cell"))
		}
		seen[formal] = struct{}{}
	}
	c.source.Functions = append(c.source.Functions, source.FunctionFormals{Function: function, Formals: append([]keyspace.Term(nil), formals...)})
	return true
}

func sourceCellAlreadyOrdered(c *Collector, cell keyspace.Term) bool {
	if c == nil {
		return false
	}
	for _, row := range c.source.Binds {
		for _, existing := range row.Cells {
			if existing == cell {
				return true
			}
		}
	}
	for _, row := range c.source.Functions {
		for _, existing := range row.Formals {
			if existing == cell {
				return true
			}
		}
	}
	return false
}

// localCellAdmission is the Collector-side row proof for a lexical Cell.
// Role predicates deliberately stop at FamilyCell; storage ownership is a
// relation between the Cell row and its Body and therefore stays here.
func localCellAdmission(c *Collector, cell keyspace.Term) bool {
	if c == nil || !validFamilyTerm(c, cell, keyspace.FamilyCell) {
		return false
	}
	ordinal := keyspace.TermOrdinal(cell)
	row, ok := c.flow.CellAt(int(ordinal - 1))
	return ordinal != 0 && ok && row.Kind == authored.CellLocal
}

func localCellInBodyAdmission(c *Collector, cell, body keyspace.Term) bool {
	if !localCellAdmission(c, cell) {
		return false
	}
	row, ok := c.flow.CellAt(int(keyspace.TermOrdinal(cell) - 1))
	return ok && row.Body == body
}
