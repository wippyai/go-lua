package declarations

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
)

// Build validates and seals the complete declaration denominator. Later
// Signatures and Contracts contribute no alternate declaration construction
// path; they only close their explicitly deferred cross-vertical laws through
// the visitors this table exposes.
func Build(input Input, counts [keyspace.FamilyCount]uint32) (Table, error) {
	declaredType := rows.NewTableBuilder[DeclaredType](keyspace.FamilyDeclaredType)
	byCell := make([]keyspace.Term, int(counts[keyspace.FamilyCell]))
	for _, row := range input.DeclaredType {
		if !hasFamily(counts, row.Cell, keyspace.FamilyCell) || !staticrole.Node(counts, row.Target) {
			return Table{}, errors.New("program/static/declarations: invalid declared type")
		}
		ordinal := keyspace.TermOrdinal(row.Cell) - 1
		if byCell[ordinal] != 0 {
			return Table{}, errors.New("program/static/declarations: duplicate declared type cell")
		}
		term, ok := declaredType.Append(row)
		if !ok {
			return Table{}, errors.New("program/static/declarations: oversized declared type table")
		}
		byCell[ordinal] = term
	}

	param := rows.NewTableBuilder[TypeParam](keyspace.FamilyTypeParam)
	for _, row := range input.TypeParam {
		if !validTypeParam(counts, row) {
			return Table{}, errors.New("program/static/declarations: invalid type parameter")
		}
		if _, ok := param.Append(row); !ok {
			return Table{}, errors.New("program/static/declarations: oversized type parameter table")
		}
	}

	var aliasParams rows.PoolBuilder[keyspace.Term]
	alias := rows.NewTableBuilder[TypeAliasRow](keyspace.FamilyTypeAlias)
	for _, row := range input.Alias {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !staticrole.Node(counts, row.Target) ||
			row.Name == 0 || !validCoordinate(row.NameCoordinate) {
			return Table{}, errors.New("program/static/declarations: invalid type alias")
		}
		params, ok := aliasParams.Append(row.Params)
		if !ok {
			return Table{}, errors.New("program/static/declarations: oversized type alias parameters")
		}
		sealed := TypeAliasRow{
			Owner: row.Owner, Target: row.Target, Name: row.Name,
			NameCoordinate: row.NameCoordinate, Params: params,
		}
		if _, ok := alias.Append(sealed); !ok {
			return Table{}, errors.New("program/static/declarations: oversized type alias table")
		}
	}

	var interfaceRefs rows.PoolBuilder[keyspace.Term]
	var members rows.PoolBuilder[InterfaceMember]
	iface := rows.NewTableBuilder[InterfaceRow](keyspace.FamilyTypeInterface)
	for _, row := range input.Interface {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || row.Name == 0 || !validCoordinate(row.NameCoordinate) {
			return Table{}, errors.New("program/static/declarations: invalid interface")
		}
		for _, ref := range row.Extends {
			if !hasFamily(counts, ref, keyspace.FamilyTypeRef) {
				return Table{}, errors.New("program/static/declarations: invalid interface extension")
			}
		}
		for _, member := range row.Members {
			if !validInterfaceMember(counts, member) {
				return Table{}, errors.New("program/static/declarations: invalid interface member")
			}
		}
		extends, ok := interfaceRefs.Append(row.Extends)
		if !ok {
			return Table{}, errors.New("program/static/declarations: oversized interface extensions")
		}
		memberSpan, ok := members.Append(row.Members)
		if !ok {
			return Table{}, errors.New("program/static/declarations: oversized interface members")
		}
		sealed := InterfaceRow{
			Owner: row.Owner, Name: row.Name, NameCoordinate: row.NameCoordinate,
			Extends: extends, Members: memberSpan,
		}
		if _, ok := iface.Append(sealed); !ok {
			return Table{}, errors.New("program/static/declarations: oversized interface table")
		}
	}

	inverse, ok := rows.NewTable(keyspace.FamilyCell, byCell)
	if !ok {
		return Table{}, errors.New("program/static/declarations: oversized declared-type cell inverse")
	}
	return Table{
		alias:          alias.Seal(),
		param:          param.Seal(),
		iface:          iface.Seal(),
		declaredType:   declaredType.Seal(),
		declaredByCell: inverse,
		aliasParams:    aliasParams.Seal(),
		interfaceRefs:  interfaceRefs.Seal(),
		members:        members.Seal(),
	}, nil
}

func validTypeParam(counts [keyspace.FamilyCount]uint32, row TypeParam) bool {
	if row.Name == 0 || !staticrole.TypeParameterOwner(counts, row.Owner) {
		return false
	}
	return row.Constraint == 0 || staticrole.Node(counts, row.Constraint)
}

func validInterfaceMember(counts [keyspace.FamilyCount]uint32, row InterfaceMember) bool {
	switch row.Kind {
	case InterfaceField:
		return hasFamily(counts, row.Field, keyspace.FamilyTypeField) && row.Name == 0 &&
			row.NameCoordinate == (source.Coordinate{}) && row.Signature == 0
	case InterfaceMethod:
		// Signatures later proves Scope is this interface. This vertical can
		// only establish the exact typed edge without importing that owner.
		return row.Field == 0 && row.Name != 0 && validCoordinate(row.NameCoordinate) &&
			hasFamily(counts, row.Signature, keyspace.FamilyTypeFunction)
	default:
		return false
	}
}

// validCoordinate admits only a present, well-formed authored span. A zero
// coordinate is the absent form and is refused wherever a name is required.
func validCoordinate(value source.Coordinate) bool {
	if value == (source.Coordinate{}) {
		return false
	}
	startLine, startColumn, endLine, endColumn := value.Parts()
	rebuilt, ok := source.CoordinateFromParts(startLine, startColumn, endLine, endColumn)
	return ok && rebuilt == value
}

func hasFamily(counts [keyspace.FamilyCount]uint32, term keyspace.Term, family keyspace.Family) bool {
	return keyspace.TermFamily(term) == family && keyspace.TermOrdinal(term) != 0 &&
		keyspace.TermOrdinal(term) <= counts[family]
}

// VisitContainment emits the concrete containment this vertical owns, in the
// canonical relation order. Lexical Cell anchors remain absent: Source and
// Flow own that geometry and close it jointly.
func (table Table) VisitContainment(attach, claimField func(parent, child keyspace.Term) bool) bool {
	if attach == nil || claimField == nil {
		return false
	}
	for parent, row := range table.alias.Terms() {
		if !attach(parent, row.Target) {
			return false
		}
	}
	for parent, row := range table.param.Terms() {
		if row.Constraint != 0 && !attach(parent, row.Constraint) {
			return false
		}
	}
	for owner, row := range table.iface.Terms() {
		for _, ref := range table.interfaceRefs.All(row.Extends) {
			if !attach(owner, ref) {
				return false
			}
		}
		for _, member := range table.members.All(row.Members) {
			switch member.Kind {
			case InterfaceField:
				if !claimField(owner, member.Field) {
					return false
				}
			case InterfaceMethod:
				if !attach(owner, member.Signature) {
					return false
				}
			default:
				return false
			}
		}
	}
	for parent, row := range table.declaredType.Terms() {
		if !attach(parent, row.Target) {
			return false
		}
	}
	return true
}
