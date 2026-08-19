package declarations

import (
	"github.com/wippyai/go-lua/analysis/program/internal/wire"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

const (
	aliasWireMin           = wire.UintWireMin * 8
	typeParamWireMin       = wire.UintWireMin * 3
	interfaceWireMin       = wire.UintWireMin * 8
	interfaceMemberWireMin = wire.UintWireMin * 8
	declaredTypeWireMin    = wire.UintWireMin * 2
)

func staticNodeFamily(family keyspace.Family) bool { return staticrole.NodeFamily(family) }
func typeRefFamily(family keyspace.Family) bool    { return family == keyspace.FamilyTypeRef }
func typeParamFamily(family keyspace.Family) bool  { return family == keyspace.FamilyTypeParam }

// WriteContent emits the exact authored scalar order of the Declarations
// vertical. Column windows and the dense Cell inverse are storage, so the
// wire carries only the authored relations, in source order.
func WriteContent(writer *framing.Writer, table Table) error {
	if writer == nil {
		return framing.ErrNilDestination
	}
	if err := writer.Count(uint64(table.alias.Count())); err != nil {
		return err
	}
	for _, row := range table.alias.All() {
		if err := writer.Uint(uint64(row.Owner)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := wire.WriteCoordinate(writer, row.NameCoordinate); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.aliasParams, row.Params); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.param.Count())); err != nil {
		return err
	}
	for _, row := range table.param.All() {
		if err := writer.Uint(uint64(row.Owner)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Constraint)); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(table.iface.Count())); err != nil {
		return err
	}
	for _, row := range table.iface.All() {
		if err := writer.Uint(uint64(row.Owner)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Name)); err != nil {
			return err
		}
		if err := wire.WriteCoordinate(writer, row.NameCoordinate); err != nil {
			return err
		}
		if err := wire.WriteTermSpan(writer, table.interfaceRefs, row.Extends); err != nil {
			return err
		}
		if err := writer.Count(uint64(table.members.Count(row.Members))); err != nil {
			return err
		}
		for _, member := range table.members.All(row.Members) {
			if err := writer.Uint(uint64(member.Kind)); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.Field)); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.Name)); err != nil {
				return err
			}
			if err := wire.WriteCoordinate(writer, member.NameCoordinate); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.Signature)); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(table.declaredType.Count())); err != nil {
		return err
	}
	for _, row := range table.declaredType.All() {
		if err := writer.Uint(uint64(row.Cell)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.Target)); err != nil {
			return err
		}
	}
	return nil
}

// Scan validates and consumes one Declarations vertical without allocating
// row slices. It is the allocation-free preflight half of Decode.
func Scan(reader *framing.Reader) error {
	_, err := decode(reader, false)
	return err
}

// Decode consumes one Declarations vertical and returns owned authored rows.
func Decode(reader *framing.Reader) (Input, error) {
	return decode(reader, true)
}

func decode(reader *framing.Reader, retain bool) (Input, error) {
	if reader == nil {
		return Input{}, framing.ErrMalformed
	}
	var input Input

	count, err := wire.Count(reader, aliasWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Alias = make([]TypeAlias, count)
	}
	for index := 0; index < count; index++ {
		owner, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || !staticNodeFamily(keyspace.TermFamily(target)) {
			return Input{}, framing.ErrMalformed
		}
		name, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		coordinate, err := wire.Coordinate(reader)
		if err != nil {
			return Input{}, err
		}
		if coordinate == (source.Coordinate{}) {
			return Input{}, framing.ErrMalformed
		}
		params, _, err := wire.TermSequence(reader, 0, retain, typeParamFamily)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Alias[index] = TypeAlias{
				Owner: owner, Target: target, Name: name,
				NameCoordinate: coordinate, Params: params,
			}
		}
	}

	count, err = wire.Count(reader, typeParamWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.TypeParam = make([]TypeParam, count)
	}
	for index := 0; index < count; index++ {
		owner, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		name, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		constraint, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if !staticrole.TypeParameterOwnerFamily(keyspace.TermFamily(owner)) ||
			(constraint != 0 && !staticNodeFamily(keyspace.TermFamily(constraint))) {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.TypeParam[index] = TypeParam{Owner: owner, Name: name, Constraint: constraint}
		}
	}

	count, err = wire.Count(reader, interfaceWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.Interface = make([]Interface, count)
	}
	for index := 0; index < count; index++ {
		owner, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return Input{}, framing.ErrMalformed
		}
		name, err := wire.Key(reader)
		if err != nil {
			return Input{}, err
		}
		coordinate, err := wire.Coordinate(reader)
		if err != nil {
			return Input{}, err
		}
		if coordinate == (source.Coordinate{}) {
			return Input{}, framing.ErrMalformed
		}
		extends, _, err := wire.TermSequence(reader, 0, retain, typeRefFamily)
		if err != nil {
			return Input{}, err
		}
		members, err := decodeMembers(reader, retain)
		if err != nil {
			return Input{}, err
		}
		if retain {
			input.Interface[index] = Interface{
				Owner: owner, Name: name, NameCoordinate: coordinate,
				Extends: extends, Members: members,
			}
		}
	}

	count, err = wire.Count(reader, declaredTypeWireMin)
	if err != nil {
		return Input{}, err
	}
	if retain {
		input.DeclaredType = make([]DeclaredType, count)
	}
	for index := 0; index < count; index++ {
		cell, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		target, err := wire.Term(reader)
		if err != nil {
			return Input{}, err
		}
		if keyspace.TermFamily(cell) != keyspace.FamilyCell || !staticNodeFamily(keyspace.TermFamily(target)) {
			return Input{}, framing.ErrMalformed
		}
		if retain {
			input.DeclaredType[index] = DeclaredType{Cell: cell, Target: target}
		}
	}
	return input, nil
}

// decodeMembers reads one interface's member sequence. The exact-xor law is
// checked here so a malformed member cannot enter the authored input at all.
func decodeMembers(reader *framing.Reader, retain bool) ([]InterfaceMember, error) {
	count, err := wire.Count(reader, interfaceMemberWireMin)
	if err != nil {
		return nil, err
	}
	var members []InterfaceMember
	if retain {
		members = make([]InterfaceMember, count)
	}
	for index := 0; index < count; index++ {
		kind, err := wire.Enum(reader, uint64(InterfaceMethod))
		if err != nil {
			return nil, err
		}
		field, err := wire.Term(reader)
		if err != nil {
			return nil, err
		}
		name, err := wire.Uint32(reader)
		if err != nil {
			return nil, err
		}
		coordinate, err := wire.Coordinate(reader)
		if err != nil {
			return nil, err
		}
		signature, err := wire.Term(reader)
		if err != nil {
			return nil, err
		}
		member := InterfaceMember{
			Kind:           InterfaceMemberKind(kind),
			Field:          field,
			Name:           keyspace.Key(name),
			NameCoordinate: coordinate,
			Signature:      signature,
		}
		switch member.Kind {
		case InterfaceField:
			if keyspace.TermFamily(field) != keyspace.FamilyTypeField || name != 0 ||
				coordinate != (source.Coordinate{}) || signature != 0 {
				return nil, framing.ErrMalformed
			}
		case InterfaceMethod:
			if field != 0 || name == 0 || coordinate == (source.Coordinate{}) ||
				keyspace.TermFamily(signature) != keyspace.FamilyTypeFunction {
				return nil, framing.ErrMalformed
			}
		}
		if retain {
			members[index] = member
		}
	}
	return members, nil
}
