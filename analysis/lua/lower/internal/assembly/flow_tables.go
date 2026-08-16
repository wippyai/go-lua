package assembly

import (
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func (c *Collector) DeclareTable(span source.Span, owner keyspace.Term) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	term := c.mint(keyspace.FamilyTable, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitTable(c.counts, term, owner); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// TableField admits a raw constructor field. For FieldExact keys, the raw
// candidate is derived from the already-authored key Term, so callers cannot
// smuggle a mismatched parallel payload.
func (c *Collector) TableField(span source.Span, table, key, values keyspace.Term, fieldKind kind.FieldKind) keyspace.Term {
	if !mutationReady(c) {
		return 0
	}
	var candidatePresent bool
	if fieldKind == kind.FieldExact {
		candidate, present := c.exactCandidate(key)
		if !present {
			return rejectTermMutationf(c, "program/lower/collector: exact TableField key has no exact candidate")
		}
		candidatePresent = true
		if !c.addExactCandidate(candidate) {
			return 0
		}
	}
	term := c.mint(keyspace.FamilyTableField, span)
	if term == 0 {
		return 0
	}
	if err := c.flow.AdmitTableField(c.counts, term, table, key, values, fieldKind, candidatePresent); err != nil {
		c.fail(err)
		return 0
	}
	return term
}

// FillTable closes a table in authored field order. Table order is a single
// shared pool; the per-table range is the only parent-local ordering witness.
func (c *Collector) FillTable(table keyspace.Term, fields []keyspace.Term) bool {
	if !mutationReady(c) {
		return false
	}
	if err := c.flow.AdmitTableFill(c.counts, table, fields); err != nil {
		c.fail(err)
		return false
	}
	return true
}
