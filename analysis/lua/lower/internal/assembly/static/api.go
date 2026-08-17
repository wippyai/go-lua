package static

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// InterfaceMember is the already-coordinated raw interface member accepted by
// the Static owner. Name payloads are still raw Source candidates; key
// resolution remains in Static.Freeze.
type InterfaceMember struct {
	Kind       programstatic.InterfaceMemberKind
	Field      keyspace.Term
	Name       string
	Coordinate programsource.Coordinate
	Signature  keyspace.Term
}

// Parameter is the already-coordinated TypeFunction parameter row.
type Parameter struct {
	Name       string
	Coordinate programsource.Coordinate
	Type       keyspace.Term
}

func (rows *Rows) InterfaceMembers(term keyspace.Term, members []InterfaceMember) error {
	if rows == nil {
		return errNilRows()
	}
	raw := make([]staticRawInterfaceMember, len(members))
	for index, member := range members {
		var name staticRawKey
		if member.Name != "" {
			var err error
			name, err = rawString(member.Name)
			if err != nil {
				return err
			}
		}
		raw[index] = staticRawInterfaceMember{kind: member.Kind, field: member.Field, name: name, coordinate: member.Coordinate, signature: member.Signature}
	}
	return rows.InterfaceMembersRaw(term, raw)
}

func (rows *Rows) TypeFunctionParameters(term keyspace.Term, params []Parameter) error {
	if rows == nil {
		return errNilRows()
	}
	raw := make([]staticRawParameter, len(params))
	for index, parameter := range params {
		var name staticRawKey
		if parameter.Name != "" {
			var err error
			name, err = rawString(parameter.Name)
			if err != nil {
				return err
			}
		}
		raw[index] = staticRawParameter{name: name, coordinate: parameter.Coordinate, typ: parameter.Type}
	}
	return rows.TypeFunctionParametersRaw(term, raw)
}

func (rows *Rows) TypeFunctionScope(term keyspace.Term) (keyspace.Term, bool) {
	if rows == nil {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if keyspace.TermFamily(term) != keyspace.FamilyTypeFunction || ordinal == 0 || int(ordinal) > len(rows.typeFunctions) {
		return 0, false
	}
	return rows.typeFunctions[ordinal-1].scope, true
}

func (rows *Rows) ReferenceResolution(term keyspace.Term) (programstatic.TypeRefResolution, bool) {
	if rows == nil {
		return programstatic.TypeRefUnresolved, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if keyspace.TermFamily(term) != keyspace.FamilyTypeRef || ordinal == 0 || int(ordinal) > len(rows.references) {
		return programstatic.TypeRefUnresolved, false
	}
	return rows.references[ordinal-1].resolution, true
}

func (rows *Rows) PublicationExists(assign keyspace.Term, pair uint32) bool {
	if rows == nil {
		return false
	}
	for _, publication := range rows.publications {
		if publication.Assign == assign && publication.Pair == pair {
			return true
		}
	}
	return false
}

func (rows *Rows) TypeParameterOwner(term keyspace.Term) (keyspace.Term, bool) {
	if rows == nil {
		return 0, false
	}
	ordinal := keyspace.TermOrdinal(term)
	if keyspace.TermFamily(term) != keyspace.FamilyTypeParam || ordinal == 0 || int(ordinal) > len(rows.params) {
		return 0, false
	}
	return rows.params[ordinal-1].owner, true
}

func (rows *Rows) OwnerAt(family keyspace.Family, index int) (keyspace.Term, bool) {
	if rows == nil || index < 0 {
		return 0, false
	}
	switch family {
	case keyspace.FamilyTypeAlias:
		if index < len(rows.aliases) {
			return rows.aliases[index].owner, true
		}
	case keyspace.FamilyTypeInterface:
		if index < len(rows.interfaces) {
			return rows.interfaces[index].owner, true
		}
	}
	return 0, false
}

func (rows *Rows) ValidTypeValueTarget(counts [keyspace.FamilyCount]uint32, target keyspace.Term) bool {
	return validStaticTypeValueTarget(&rows.staticRows, counts, target)
}

func (rows *Rows) Freeze(preimage programsource.Preimage, counts [keyspace.FamilyCount]uint32) (programstatic.Input, error) {
	if rows == nil {
		return programstatic.Input{}, errNilRows()
	}
	return rows.freeze(preimage, counts)
}

func errNilRows() error {
	return fmt.Errorf("program/lower/collector: nil Static rows")
}
