package static

import (
	"errors"
	"math"

	"github.com/wippyai/go-lua/program/internal/canonical"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
	staticrole "github.com/wippyai/go-lua/program/static/role"
)

// compactDeclarations owns the complete declaration denominator. Later
// Signatures and Contracts contribute no alternate declaration construction
// path; they only close their explicitly deferred cross-vertical laws.
func compactDeclarations(component *Component, counts [keyspace.FamilyCount]uint32, input DeclarationsInput) error {
	store := &component.declarations
	// This dense inverse is a query derivative, never a second semantic
	// authority. The authored rows below remain the sole declaration relation.
	store.declaredByCell = make([]keyspace.Term, int(counts[keyspace.FamilyCell]))
	for index, row := range input.DeclaredType {
		if !hasFamily(counts, row.Cell, keyspace.FamilyCell) || !staticrole.Node(counts, row.Target) {
			return errors.New("program/static: invalid declared type")
		}
		ordinal := keyspace.TermOrdinal(row.Cell) - 1
		if store.declaredByCell[ordinal] != 0 {
			return errors.New("program/static: duplicate declared type cell")
		}
		term := keyspace.MakeTerm(keyspace.FamilyDeclaredType, uint32(index+1))
		store.declaredByCell[ordinal] = term
		store.declaredTypes = append(store.declaredTypes, declaredTypeRow{cell: row.Cell, target: row.Target})
	}
	for _, row := range input.TypeParam {
		if !validTypeParam(counts, row) {
			return errors.New("program/static: invalid type parameter")
		}
		store.params = append(store.params, row)
	}
	for _, row := range input.Alias {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || !staticrole.Node(counts, row.Target) ||
			row.Name == 0 || !validCoordinate(row.NameCoordinate) {
			return errors.New("program/static: invalid type alias")
		}
		params, ok := appendTerms(&store.aliasParams, row.Params)
		if !ok {
			return errors.New("program/static: oversized type alias parameters")
		}
		store.aliases = append(store.aliases, typeAliasRow{
			owner: row.Owner, target: row.Target, name: row.Name, coordinate: row.NameCoordinate, params: params,
		})
	}
	for _, row := range input.Interface {
		if !hasFamily(counts, row.Owner, keyspace.FamilyBody) || row.Name == 0 || !validCoordinate(row.NameCoordinate) {
			return errors.New("program/static: invalid interface")
		}
		for _, ref := range row.Extends {
			if !hasFamily(counts, ref, keyspace.FamilyTypeRef) {
				return errors.New("program/static: invalid interface extension")
			}
		}
		extends, ok := appendTerms(&store.interfaceRefs, row.Extends)
		if !ok {
			return errors.New("program/static: oversized interface extensions")
		}
		members, ok := appendInterfaceMembers(&store.members, row.Members)
		if !ok {
			return errors.New("program/static: oversized interface members")
		}
		for _, member := range row.Members {
			if !validInterfaceMember(counts, member) {
				return errors.New("program/static: invalid interface member")
			}
		}
		store.interfaces = append(store.interfaces, interfaceRow{
			owner: row.Owner, name: row.Name, coordinate: row.NameCoordinate, extends: extends, members: members,
		})
	}
	return nil
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
		// Signatures later prove Scope == this interface. This vertical can only
		// establish the exact typed edge without importing that future owner.
		return row.Field == 0 && row.Name != 0 && validCoordinate(row.NameCoordinate) &&
			hasFamily(counts, row.Signature, keyspace.FamilyTypeFunction)
	default:
		return false
	}
}

func validCoordinate(value source.Coordinate) bool {
	if value == (source.Coordinate{}) {
		return false
	}
	startLine, startCol, endLine, endCol := value.Parts()
	copy, ok := source.CoordinateFromParts(startLine, startCol, endLine, endCol)
	return ok && copy == value
}

func appendInterfaceMembers(pool *[]interfaceMemberRow, input []InterfaceMember) (poolRange, bool) {
	start := len(*pool)
	if uint64(start)+uint64(len(input)) > uint64(math.MaxUint32) {
		return poolRange{}, false
	}
	for _, row := range input {
		*pool = append(*pool, interfaceMemberRow{
			kind: row.Kind, field: row.Field, name: row.Name, coordinate: row.NameCoordinate, signature: row.Signature,
		})
	}
	return poolRange{Start: uint32(start), End: uint32(len(*pool))}, true
}

// writeDeclarationsContent owns Static declaration records. The pools are
// expanded in source relation order so storage offsets and inverse indexes do
// not participate in authored identity.
func writeDeclarationsContent(writer *canonical.Writer, store declarationStore) error {
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

// writeCoordinateContent encodes the four exact authored Coordinate fields.
// It is owned by the declaration vertical because declarations introduce the
// shared Static spelling-coordinate representation used by signatures.
func writeCoordinateContent(writer *canonical.Writer, coordinate source.Coordinate) error {
	startLine, startColumn, endLine, endColumn := coordinate.Parts()
	if err := writer.Uint(uint64(startLine)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(startColumn)); err != nil {
		return err
	}
	if err := writer.Uint(uint64(endLine)); err != nil {
		return err
	}
	return writer.Uint(uint64(endColumn))
}

// emitDeclarationsContainment owns authored declaration syntax. Lexical Cell
// anchors remain absent: Source/Flow own that geometry and close it jointly.
func emitDeclarationsContainment(component *Component, check *containment) bool {
	store := &component.declarations
	for index, row := range store.aliases {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeAlias, uint32(index+1)), row.target) {
			return false
		}
	}
	for index, row := range store.params {
		if row.Constraint != 0 && !check.attach(keyspace.MakeTerm(keyspace.FamilyTypeParam, uint32(index+1)), row.Constraint) {
			return false
		}
	}
	for index, row := range store.interfaces {
		owner := keyspace.MakeTerm(keyspace.FamilyTypeInterface, uint32(index+1))
		for _, ref := range store.interfaceRefs[row.extends.Start:row.extends.End] {
			if !check.attach(owner, ref) {
				return false
			}
		}
		for _, member := range store.members[row.members.Start:row.members.End] {
			switch member.kind {
			case InterfaceField:
				if !check.claimField(owner, member.field) {
					return false
				}
			case InterfaceMethod:
				if !check.attach(owner, member.signature) {
					return false
				}
			default:
				return false
			}
		}
	}
	for index, row := range store.declaredTypes {
		if !check.attach(keyspace.MakeTerm(keyspace.FamilyDeclaredType, uint32(index+1)), row.target) {
			return false
		}
	}
	return true
}
