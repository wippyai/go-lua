package collector

import (
	"github.com/wippyai/go-lua/program/flow"
	"github.com/wippyai/go-lua/program/flow/kind"
	flowrole "github.com/wippyai/go-lua/program/flow/role"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

func (w FlowTablesWriter) DeclareTable(span source.Span, owner keyspace.Term) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, owner, keyspace.FamilyBody) {
		return rejectTermMutationf(c, "program/lower/collector: invalid Table owner")
	}
	term := c.mint(keyspace.FamilyTable, span)
	if term == 0 {
		return 0
	}
	c.flow.tables.tables = append(c.flow.tables.tables, flow.Table{Owner: owner})
	c.flow.tables.tableFilled = append(c.flow.tables.tableFilled, false)
	return term
}

// TableField admits a raw constructor field. For FieldExact keys, the raw
// candidate is derived from the already-authored key Term, so callers cannot
// smuggle a mismatched parallel payload.
func (w FlowTablesWriter) TableField(span source.Span, table, key, values keyspace.Term, fieldKind kind.FieldKind) keyspace.Term {
	c := w.collector
	if !mutationReady(c) {
		return 0
	}
	if !validFamilyTerm(c, table, keyspace.FamilyTable) || !flowrole.FieldSourceFamily(c.counts, key, fieldKind) || !validFamilyTerm(c, values, keyspace.FamilyValues) ||
		fieldKind < kind.FieldList || fieldKind > kind.FieldKey {
		return rejectTermMutationf(c, "program/lower/collector: invalid TableField admission")
	}
	if fieldKind == kind.FieldExact {
		access := FlowAccessWriter{collector: c}
		candidate, present := access.exactCandidate(key)
		if !present {
			return rejectTermMutationf(c, "program/lower/collector: exact TableField key has no exact candidate")
		}
		if !access.addExactCandidate(candidate) {
			return 0
		}
	}
	term := c.mint(keyspace.FamilyTableField, span)
	if term == 0 {
		return 0
	}
	c.flow.tables.tableFields = append(c.flow.tables.tableFields, flow.Field{Table: table, Key: key, Values: values, Kind: fieldKind})
	return term
}

// FillTable closes a table in authored field order. Table order is a single
// shared pool; the per-table range is the only parent-local ordering witness.
func (w FlowTablesWriter) FillTable(table keyspace.Term, fields []keyspace.Term) bool {
	c := w.collector
	if !mutationReady(c) {
		return false
	}
	if !validFamilyTerm(c, table, keyspace.FamilyTable) || len(c.flow.tables.tables) < int(keyspace.TermOrdinal(table)) || len(c.flow.tables.tableFilled) != len(c.flow.tables.tables) {
		return rejectMutationf(c, "program/lower/collector: invalid Table fill")
	}
	row := &c.flow.tables.tables[keyspace.TermOrdinal(table)-1]
	if c.flow.tables.tableFilled[keyspace.TermOrdinal(table)-1] {
		return rejectMutationf(c, "program/lower/collector: Table filled twice")
	}
	for _, field := range fields {
		if !validFamilyTerm(c, field, keyspace.FamilyTableField) || keyspace.TermOrdinal(field) > uint32(len(c.flow.tables.tableFields)) || c.flow.tables.tableFields[keyspace.TermOrdinal(field)-1].Table != table {
			return rejectMutationf(c, "program/lower/collector: invalid Table field")
		}
	}
	r, ok := appendTerms(&c.flow.tables.tableOrder, fields)
	if !ok {
		return rejectMutationf(c, "program/lower/collector: Table field range overflow")
	}
	row.Fields = r
	c.flow.tables.tableFilled[keyspace.TermOrdinal(table)-1] = true
	return true
}
