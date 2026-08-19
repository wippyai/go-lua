package operators

import (
	"errors"

	flowrole "github.com/wippyai/go-lua/analysis/program/flow/role"
	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// Build validates and seals authored operator rows. It retains no input
// slices and returns the only operator table that Static may publish.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	typeOf := rows.NewTableBuilder[TypeOf](keyspace.FamilyTypeOf)
	for _, row := range input.TypeOf {
		if !staticrole.ScopeHandle(counts, row.Scope) || !flowrole.ValueOccurrence(counts, row.Operand) {
			return Table{}, errors.New("program/static/operators: invalid typeof scope or operand")
		}
		if _, ok := typeOf.Append(row); !ok {
			return Table{}, errors.New("program/static/operators: oversized typeof table")
		}
	}
	keyOf := rows.NewTableBuilder[KeyOf](keyspace.FamilyTypeKeyOf)
	for _, row := range input.KeyOf {
		if !staticrole.Node(counts, row.Inner) {
			return Table{}, errors.New("program/static/operators: invalid keyof child")
		}
		if _, ok := keyOf.Append(row); !ok {
			return Table{}, errors.New("program/static/operators: oversized keyof table")
		}
	}
	indexAccess := rows.NewTableBuilder[IndexAccess](keyspace.FamilyTypeIndexAccess)
	for _, row := range input.IndexAccess {
		if !staticrole.Node(counts, row.Object) || !staticrole.Node(counts, row.Index) {
			return Table{}, errors.New("program/static/operators: invalid indexed-access child")
		}
		if _, ok := indexAccess.Append(row); !ok {
			return Table{}, errors.New("program/static/operators: oversized indexed-access table")
		}
	}
	conditional := rows.NewTableBuilder[Conditional](keyspace.FamilyTypeConditional)
	for _, row := range input.Conditional {
		if !staticrole.Node(counts, row.Check) || !staticrole.Node(counts, row.Extends) ||
			!staticrole.Node(counts, row.Then) || !staticrole.Node(counts, row.Else) {
			return Table{}, errors.New("program/static/operators: invalid conditional child")
		}
		if _, ok := conditional.Append(row); !ok {
			return Table{}, errors.New("program/static/operators: oversized conditional table")
		}
	}
	return Table{
		typeOf:      typeOf.Seal(),
		keyOf:       keyOf.Seal(),
		indexAccess: indexAccess.Seal(),
		conditional: conditional.Seal(),
	}, nil
}

// Count reports the number of rows in one operator family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypeOf:
		return table.typeOf.Count()
	case keyspace.FamilyTypeKeyOf:
		return table.keyOf.Count()
	case keyspace.FamilyTypeIndexAccess:
		return table.indexAccess.Count()
	case keyspace.FamilyTypeConditional:
		return table.conditional.Count()
	default:
		return 0
	}
}

// VisitContainment visits the static children owned by this table in
// canonical family order. The callback is a composition boundary only: no
// child row or mutable storage escapes the operator owner.
func (table Table) VisitContainment(visit func(parent, child keyspace.Term) bool) bool {
	if visit == nil {
		return false
	}
	for parent, row := range table.keyOf.Terms() {
		if !visit(parent, row.Inner) {
			return false
		}
	}
	for parent, row := range table.indexAccess.Terms() {
		if !visit(parent, row.Object) || !visit(parent, row.Index) {
			return false
		}
	}
	for parent, row := range table.conditional.Terms() {
		if !visit(parent, row.Check) || !visit(parent, row.Extends) ||
			!visit(parent, row.Then) || !visit(parent, row.Else) {
			return false
		}
	}
	return true
}
