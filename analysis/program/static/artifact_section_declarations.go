package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	staticrole "github.com/wippyai/go-lua/analysis/program/static/role"
	"github.com/wippyai/go-lua/internal/framing"
)

func (decoder *staticArtifactDecoder) declarations(output *DeclarationsInput) error {
	if !decoder.probing && !decoder.preflighted {
		if err := decoder.preflightDeclarations(); err != nil {
			return err
		}
	}
	count, err := decoder.count(staticArtifactAliasWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Alias = make([]TypeAlias, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody || !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		params, err := decoder.termSequenceConstraint(0, staticArtifactTypeParamTerm)
		if err != nil {
			return err
		}
		if !decoder.probing {
			output.Alias[index] = TypeAlias{Owner: owner, Target: target, Name: name, NameCoordinate: coordinate, Params: params}
		}
	}

	count, err = decoder.count(staticArtifactTypeParamWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.TypeParam = make([]TypeParam, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		constraint, err := decoder.term()
		if err != nil {
			return err
		}
		if !staticrole.TypeParameterOwnerFamily(keyspace.TermFamily(owner)) {
			return errInvalidArtifactSection
		}
		if !staticrole.TypeParameterOwnerFamily(keyspace.TermFamily(owner)) || (constraint != 0 && !validDecodedTerm(constraint, staticArtifactStaticNodeTerm)) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.TypeParam[index] = TypeParam{Owner: owner, Name: name, Constraint: constraint}
		}
	}

	count, err = decoder.count(staticArtifactInterfaceWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.Interface = make([]Interface, count)
	}
	for index := 0; index < count; index++ {
		owner, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(owner) != keyspace.FamilyBody {
			return errInvalidArtifactSection
		}
		name, err := decoder.key()
		if err != nil {
			return err
		}
		coordinate, err := decoder.coordinate()
		if err != nil || coordinate == (source.Coordinate{}) {
			if err != nil {
				return err
			}
			return errInvalidArtifactSection
		}
		extends, err := decoder.termSequenceConstraint(0, staticArtifactTypeRefTerm)
		if err != nil {
			return err
		}
		memberCount, err := decoder.count(staticArtifactInterfaceMemberWireMin)
		if err != nil {
			return err
		}
		var members []InterfaceMember
		if !decoder.probing {
			members = make([]InterfaceMember, memberCount)
		}
		for memberIndex := 0; memberIndex < memberCount; memberIndex++ {
			kind, err := decoder.enum(uint64(InterfaceMethod))
			if err != nil {
				return err
			}
			field, err := decoder.term()
			if err != nil {
				return err
			}
			memberName, err := decoder.uint32()
			if err != nil {
				return err
			}
			coordinate, err := decoder.coordinate()
			if err != nil {
				return err
			}
			signature, err := decoder.term()
			if err != nil {
				return err
			}
			member := InterfaceMember{
				Kind:           InterfaceMemberKind(kind),
				Field:          field,
				Name:           keyspace.Key(memberName),
				NameCoordinate: coordinate,
				Signature:      signature,
			}
			switch member.Kind {
			case InterfaceField:
				if keyspace.TermFamily(field) != keyspace.FamilyTypeField || memberName != 0 || coordinate != (source.Coordinate{}) || signature != 0 {
					return errInvalidArtifactSection
				}
			case InterfaceMethod:
				if field != 0 || memberName == 0 || coordinate == (source.Coordinate{}) || keyspace.TermFamily(signature) != keyspace.FamilyTypeFunction {
					return errInvalidArtifactSection
				}
			}
			if !decoder.probing {
				members[memberIndex] = member
			}
		}
		if !decoder.probing {
			output.Interface[index] = Interface{Owner: owner, Name: name, NameCoordinate: coordinate, Extends: extends, Members: members}
		}
	}

	count, err = decoder.count(staticArtifactDeclaredTypeWireMin)
	if err != nil {
		return err
	}
	if !decoder.probing {
		output.DeclaredType = make([]DeclaredType, count)
	}
	for index := 0; index < count; index++ {
		cell, err := decoder.term()
		if err != nil {
			return err
		}
		target, err := decoder.term()
		if err != nil {
			return err
		}
		if keyspace.TermFamily(cell) != keyspace.FamilyCell || !validDecodedTerm(target, staticArtifactStaticNodeTerm) {
			return errInvalidArtifactSection
		}
		if !decoder.probing {
			output.DeclaredType[index] = DeclaredType{Cell: cell, Target: target}
		}
	}
	return nil
}

// writeDeclarationsContent owns Static declaration records. The pools are
// expanded in source relation order so storage offsets and inverse indexes do
// not participate in authored identity.
func writeDeclarationsContent(writer *framing.Writer, store declarationStore) error {
	if err := writer.Count(uint64(len(store.aliases))); err != nil {
		return err
	}
	for _, row := range store.aliases {
		if err := writer.Uint(uint64(row.owner)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.name)); err != nil {
			return err
		}
		if err := writeCoordinateContent(writer, row.coordinate); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.aliasParams[row.params.Start:row.params.End]); err != nil {
			return err
		}
	}
	if err := writer.Count(uint64(len(store.params))); err != nil {
		return err
	}
	for _, row := range store.params {
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
	if err := writer.Count(uint64(len(store.interfaces))); err != nil {
		return err
	}
	for _, row := range store.interfaces {
		if err := writer.Uint(uint64(row.owner)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.name)); err != nil {
			return err
		}
		if err := writeCoordinateContent(writer, row.coordinate); err != nil {
			return err
		}
		if err := writeTypeTermsContent(writer, store.interfaceRefs[row.extends.Start:row.extends.End]); err != nil {
			return err
		}
		members := store.members[row.members.Start:row.members.End]
		if err := writer.Count(uint64(len(members))); err != nil {
			return err
		}
		for _, member := range members {
			if err := writer.Uint(uint64(member.kind)); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.field)); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.name)); err != nil {
				return err
			}
			if err := writeCoordinateContent(writer, member.coordinate); err != nil {
				return err
			}
			if err := writer.Uint(uint64(member.signature)); err != nil {
				return err
			}
		}
	}
	if err := writer.Count(uint64(len(store.declaredTypes))); err != nil {
		return err
	}
	for _, row := range store.declaredTypes {
		if err := writer.Uint(uint64(row.cell)); err != nil {
			return err
		}
		if err := writer.Uint(uint64(row.target)); err != nil {
			return err
		}
	}
	return nil
}
