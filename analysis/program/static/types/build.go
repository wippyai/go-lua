package types

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/rows"
)

// Build validates and seals the authored typed static-syntax relations. Field
// membership is intentionally deferred to the enclosing owner's one combined
// containment seal, because an Interface, owned by Declarations, can also
// claim a Field.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	if !validLeafRows(input) {
		return Table{}, errors.New("program/static/types: invalid primitive, literal, or field")
	}
	for _, row := range input.Optional {
		if !staticrole.Node(counts, row.Inner) {
			return Table{}, errors.New("program/static/types: invalid optional child")
		}
	}
	for _, row := range input.Union {
		if !validCompound(counts, row.Members) {
			return Table{}, errors.New("program/static/types: invalid compound child")
		}
	}
	for _, row := range input.Intersection {
		if !validCompound(counts, row.Members) {
			return Table{}, errors.New("program/static/types: invalid compound child")
		}
	}
	for _, row := range input.Generic {
		if !keyspace.ValidTerm(row.Base, keyspace.FamilyTypeRef, int(counts[keyspace.FamilyTypeRef])) || len(row.Args) == 0 {
			return Table{}, errors.New("program/static/types: invalid generic base or arity")
		}
		for _, arg := range row.Args {
			if !staticrole.Node(counts, arg) {
				return Table{}, errors.New("program/static/types: invalid generic argument")
			}
		}
	}
	for _, row := range input.Array {
		if !staticrole.Node(counts, row.Element) {
			return Table{}, errors.New("program/static/types: invalid array element")
		}
	}
	for _, row := range input.Map {
		if !staticrole.Node(counts, row.Key) || !staticrole.Node(counts, row.Value) {
			return Table{}, errors.New("program/static/types: invalid map child")
		}
	}
	for _, row := range input.Record {
		for _, field := range row.Fields {
			if !keyspace.ValidTerm(field, keyspace.FamilyTypeField, int(counts[keyspace.FamilyTypeField])) {
				return Table{}, errors.New("program/static/types: invalid record field")
			}
		}
	}
	return seal(input)
}

// seal places the variable-width children into their two shared columns and
// hands every relation over as an immutable table.
func seal(input Input) (Table, error) {
	var terms rows.PoolBuilder[keyspace.Term]

	union := rows.NewTableBuilder[MembersRow](keyspace.FamilyTypeUnion)
	for _, row := range input.Union {
		span, ok := terms.Append(row.Members)
		if !ok {
			return Table{}, errors.New("program/static/types: oversized union")
		}
		if _, ok := union.Append(MembersRow{Members: span}); !ok {
			return Table{}, errors.New("program/static/types: oversized union table")
		}
	}
	intersection := rows.NewTableBuilder[MembersRow](keyspace.FamilyTypeIntersection)
	for _, row := range input.Intersection {
		span, ok := terms.Append(row.Members)
		if !ok {
			return Table{}, errors.New("program/static/types: oversized intersection")
		}
		if _, ok := intersection.Append(MembersRow{Members: span}); !ok {
			return Table{}, errors.New("program/static/types: oversized intersection table")
		}
	}
	generic := rows.NewTableBuilder[GenericRow](keyspace.FamilyTypeGeneric)
	for _, row := range input.Generic {
		span, ok := terms.Append(row.Args)
		if !ok {
			return Table{}, errors.New("program/static/types: oversized generic")
		}
		if _, ok := generic.Append(GenericRow{Base: row.Base, Args: span}); !ok {
			return Table{}, errors.New("program/static/types: oversized generic table")
		}
	}
	record := rows.NewTableBuilder[RecordRow](keyspace.FamilyTypeRecord)
	for _, row := range input.Record {
		span, ok := terms.Append(row.Fields)
		if !ok {
			return Table{}, errors.New("program/static/types: oversized record")
		}
		if _, ok := record.Append(RecordRow{Fields: span, ReadOnly: row.ReadOnly}); !ok {
			return Table{}, errors.New("program/static/types: oversized record table")
		}
	}

	table := Table{
		union:        union.Seal(),
		intersection: intersection.Seal(),
		generic:      generic.Seal(),
		record:       record.Seal(),
		terms:        terms.Seal(),
	}
	var ok bool
	if table.primitive, ok = rows.NewTable(keyspace.FamilyTypePrimitive, input.Primitive); !ok {
		return Table{}, errors.New("program/static/types: oversized primitive table")
	}
	if table.literal, ok = rows.NewTable(keyspace.FamilyTypeLiteral, input.Literal); !ok {
		return Table{}, errors.New("program/static/types: oversized literal table")
	}
	if table.optional, ok = rows.NewTable(keyspace.FamilyTypeOptional, input.Optional); !ok {
		return Table{}, errors.New("program/static/types: oversized optional table")
	}
	if table.array, ok = rows.NewTable(keyspace.FamilyTypeArray, input.Array); !ok {
		return Table{}, errors.New("program/static/types: oversized array table")
	}
	if table.mapType, ok = rows.NewTable(keyspace.FamilyTypeMap, input.Map); !ok {
		return Table{}, errors.New("program/static/types: oversized map table")
	}
	if table.field, ok = rows.NewTable(keyspace.FamilyTypeField, input.Field); !ok {
		return Table{}, errors.New("program/static/types: oversized field table")
	}
	return table, nil
}

func validLeafRows(input Input) bool {
	for _, row := range input.Primitive {
		if !row.Kind.Valid() {
			return false
		}
	}
	for _, row := range input.Literal {
		switch row.Kind {
		case keyspace.LiteralBool, keyspace.LiteralInteger, keyspace.LiteralString:
			if row.Exact == 0 || row.FloatBits != 0 {
				return false
			}
		case keyspace.LiteralFloat:
			if row.Exact != 0 {
				return false
			}
		default:
			return false
		}
	}
	for _, row := range input.Field {
		if row.Key == 0 {
			return false
		}
	}
	return true
}

// validCompound is the one arity and child law shared by Union and
// Intersection: both are compound relations of at least two authored nodes.
func validCompound(counts [keyspace.FamilyCount]uint32, members []keyspace.Term) bool {
	if len(members) < 2 {
		return false
	}
	for _, term := range members {
		if !staticrole.Node(counts, term) {
			return false
		}
	}
	return true
}

// VisitContainment emits the concrete containment this vertical owns, in the
// canonical relation order. attach carries a parent-to-child type edge;
// claimField carries Record-to-Field membership, which is a distinct
// ownership relation rather than a type edge. The callbacks are a composition
// boundary only: no row or storage escapes the owner.
func (table Table) VisitContainment(attach, claimField func(parent, child keyspace.Term) bool) bool {
	if attach == nil || claimField == nil {
		return false
	}
	for parent, row := range table.optional.Terms() {
		if !attach(parent, row.Inner) {
			return false
		}
	}
	for _, compound := range []rows.Table[MembersRow]{table.union, table.intersection} {
		for parent, row := range compound.Terms() {
			for _, child := range table.terms.All(row.Members) {
				if !attach(parent, child) {
					return false
				}
			}
		}
	}
	for parent, row := range table.generic.Terms() {
		if !attach(parent, row.Base) {
			return false
		}
		for _, child := range table.terms.All(row.Args) {
			if !attach(parent, child) {
				return false
			}
		}
	}
	for parent, row := range table.array.Terms() {
		if !attach(parent, row.Element) {
			return false
		}
	}
	for parent, row := range table.mapType.Terms() {
		if !attach(parent, row.Key) || !attach(parent, row.Value) {
			return false
		}
	}
	for parent, row := range table.field.Terms() {
		if !attach(parent, row.Type) {
			return false
		}
	}
	for owner, row := range table.record.Terms() {
		for _, field := range table.terms.All(row.Fields) {
			if !claimField(owner, field) {
				return false
			}
		}
	}
	return true
}
