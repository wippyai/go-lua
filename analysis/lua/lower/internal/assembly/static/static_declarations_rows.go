package static

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

// AliasDeclare/Params/Target implement the three-phase alias construction
// without retaining an incomplete public row. Params and target are each
// one-shot fills, including explicit empty parameter lists.
func (rows *staticRows) AliasDeclare(term, owner keyspace.Term, name string, coordinate source.Coordinate) error {
	if rows == nil || !validCoordinateOrZero(coordinate) || coordinate == (source.Coordinate{}) {
		return errors.New("program/lower/collector: invalid alias declaration")
	}
	if err := requireFamily(owner, keyspace.FamilyBody); err != nil {
		return err
	}
	if err := requireFamily(term, keyspace.FamilyTypeAlias); err != nil {
		return err
	}
	if keyspace.TermOrdinal(term) != uint32(len(rows.aliases)+1) {
		return errors.New("program/lower/collector: noncanonical alias ordinal")
	}
	key, err := rawString(name)
	if err != nil {
		return err
	}
	rows.aliases = append(rows.aliases, staticRawAlias{owner: owner, name: key, coordinate: coordinate})
	return nil
}

func (rows *staticRows) AliasParams(term keyspace.Term, params []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeAlias, len(rows.aliases))
	if err != nil {
		return err
	}
	if rows.aliases[index].paramsSet {
		return errors.New("program/lower/collector: alias parameters filled twice")
	}
	rows.aliases[index].params = append([]keyspace.Term(nil), params...)
	rows.aliases[index].paramsSet = true
	return nil
}

func (rows *staticRows) AliasTarget(term, target keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeAlias, len(rows.aliases))
	if err != nil {
		return err
	}
	if rows.aliases[index].targetSet || target == 0 {
		return errors.New("program/lower/collector: alias target filled twice or missing")
	}
	rows.aliases[index].target, rows.aliases[index].targetSet = target, true
	return nil
}

// InterfaceDeclare/Extends/Members preserve member order and exact variant
// shape. A field member has no name; a method member has no Field.
func (rows *staticRows) InterfaceDeclare(term, owner keyspace.Term, name string, coordinate source.Coordinate) error {
	if rows == nil || coordinate == (source.Coordinate{}) || !validCoordinateOrZero(coordinate) {
		return errors.New("program/lower/collector: invalid interface declaration")
	}
	if err := requireFamily(owner, keyspace.FamilyBody); err != nil {
		return err
	}
	if err := requireFamily(term, keyspace.FamilyTypeInterface); err != nil {
		return err
	}
	if keyspace.TermOrdinal(term) != uint32(len(rows.interfaces)+1) {
		return errors.New("program/lower/collector: noncanonical interface ordinal")
	}
	key, err := rawString(name)
	if err != nil {
		return err
	}
	rows.interfaces = append(rows.interfaces, staticRawInterface{owner: owner, name: key, coordinate: coordinate})
	return nil
}

func (rows *staticRows) InterfaceExtends(term keyspace.Term, extends []keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeInterface, len(rows.interfaces))
	if err != nil {
		return err
	}
	if rows.interfaces[index].extendsSet {
		return errors.New("program/lower/collector: interface extends filled twice")
	}
	rows.interfaces[index].extends = append([]keyspace.Term(nil), extends...)
	rows.interfaces[index].extendsSet = true
	return nil
}

// InterfaceMemberRaw is the preferred construction API because a member name
// must remain a payload until Source freezes. It mirrors InterfaceMembers but
// does not accept a numeric Key.
func (rows *staticRows) InterfaceMembersRaw(term keyspace.Term, members []staticRawInterfaceMember) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeInterface, len(rows.interfaces))
	if err != nil {
		return err
	}
	if rows.interfaces[index].membersSet {
		return errors.New("program/lower/collector: interface members filled twice")
	}
	copyRows := make([]staticRawInterfaceMember, len(members))
	for i, member := range members {
		if !validCoordinateOrZero(member.coordinate) {
			return errors.New("program/lower/collector: invalid interface member coordinate")
		}
		if member.kind == programstatic.InterfaceField {
			if member.field == 0 || member.name.present || member.signature != 0 || member.coordinate != (source.Coordinate{}) {
				return errors.New("program/lower/collector: invalid interface field member")
			}
		} else if member.kind == programstatic.InterfaceMethod {
			if member.field != 0 || !member.name.present || member.signature == 0 || member.coordinate == (source.Coordinate{}) {
				return errors.New("program/lower/collector: invalid interface method member")
			}
		} else {
			return errors.New("program/lower/collector: invalid interface member kind")
		}
		copyRows[i] = member
	}
	rows.interfaces[index].membersRaw = copyRows
	rows.interfaces[index].membersSet = true
	return nil
}

// TypeParamDeclare/Fill implement the one-shot constraint attachment.
func (rows *staticRows) TypeParamDeclare(term, owner keyspace.Term, name string) error {
	if rows == nil || term == 0 || keyspace.TermOrdinal(term) != uint32(len(rows.params)+1) {
		return errors.New("program/lower/collector: invalid type parameter declaration")
	}
	if keyspace.TermFamily(term) != keyspace.FamilyTypeParam {
		return errors.New("program/lower/collector: invalid type parameter family")
	}
	if keyspace.TermFamily(owner) != keyspace.FamilyTypeAlias && keyspace.TermFamily(owner) != keyspace.FamilyTypeFunction && keyspace.TermFamily(owner) != keyspace.FamilyFunction {
		return errors.New("program/lower/collector: invalid type parameter owner")
	}
	key, err := rawString(name)
	if err != nil {
		return err
	}
	rows.params = append(rows.params, staticRawParam{owner: owner, name: key})
	return nil
}

func (rows *staticRows) TypeParamFill(term, constraint keyspace.Term) error {
	index, err := denseOrdinal(term, keyspace.FamilyTypeParam, len(rows.params))
	if err != nil {
		return err
	}
	if rows.params[index].filled {
		return errors.New("program/lower/collector: type parameter filled twice")
	}
	rows.params[index].constraint, rows.params[index].filled = constraint, true
	return nil
}

func (rows *staticRows) DeclaredType(term, cell, target keyspace.Term) error {
	if rows == nil || keyspace.TermFamily(term) != keyspace.FamilyDeclaredType || keyspace.TermOrdinal(term) != uint32(len(rows.declared)+1) {
		return errors.New("program/lower/collector: invalid declared type term")
	}
	if cell == 0 || target == 0 {
		return errors.New("program/lower/collector: incomplete declared type")
	}
	rows.declared = append(rows.declared, programstatic.DeclaredType{Cell: cell, Target: target})
	return nil
}
