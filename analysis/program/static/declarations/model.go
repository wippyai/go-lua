// Package declarations owns the authored static declaration relations: type
// aliases, type parameters, interfaces with their members, and the binding
// from a canonical lexical Cell to its declared static type.
//
// The package is independent of the enclosing Static component. It validates
// and seals its own rows, exposes immutable queries, and hands the resulting
// table back to Static as a value. It deliberately does not form a generic
// declaration node/edge vocabulary: every ownership and ordering law stays
// visible as its own typed relation.
package declarations

import (
	"github.com/wippyai/go-lua/analysis/program/internal/rows"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TypeAlias is one authored named static type binding.
type TypeAlias struct {
	Owner          keyspace.Term
	Target         keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Params         []keyspace.Term
}

// TypeParam has no secondary coordinate: the owning Term span is its name.
// Its owner may be a TypeAlias, a Signature TypeFunction, or Flow's authored
// Function family.
type TypeParam struct {
	Owner      keyspace.Term
	Name       keyspace.Key
	Constraint keyspace.Term
}

type Interface struct {
	Owner          keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Extends        []keyspace.Term
	Members        []InterfaceMember
}

type InterfaceMemberKind uint8

const (
	InterfaceField InterfaceMemberKind = iota + 1
	InterfaceMethod
)

// InterfaceMember is exact-xor: Field members populate Field only; Method
// members populate Name, NameCoordinate, and Signature only.
type InterfaceMember struct {
	Kind           InterfaceMemberKind
	Field          keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Signature      keyspace.Term
}

// DeclaredType is one authored binding from a canonical lexical Cell to its
// declared static type. Cell identity remains owned by Flow/Source; this
// relation owns neither a name nor a coordinate and must not reconstruct one.
type DeclaredType struct {
	Cell   keyspace.Term
	Target keyspace.Term
}

// Input is the complete authored declaration denominator.
type Input struct {
	Alias        []TypeAlias
	TypeParam    []TypeParam
	Interface    []Interface
	DeclaredType []DeclaredType
}

// TypeAliasRow is the sealed form of a TypeAlias: its parameters live in the
// shared alias-parameter column and the row keeps only their window.
type TypeAliasRow struct {
	Owner          keyspace.Term
	Target         keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Params         rows.Span
}

// InterfaceRow is the sealed form of an Interface.
type InterfaceRow struct {
	Owner          keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Extends        rows.Span
	Members        rows.Span
}

// Table is the sealed immutable declaration relation set.
//
// declaredByCell is a dense inverse keyed by the canonical Cell ordinal. It
// is a query derivative, never a second semantic authority: the DeclaredType
// table below remains the sole declaration relation.
type Table struct {
	alias          rows.Table[TypeAliasRow]
	param          rows.Table[TypeParam]
	iface          rows.Table[InterfaceRow]
	declaredType   rows.Table[DeclaredType]
	declaredByCell rows.Table[keyspace.Term]
	aliasParams    rows.Pool[keyspace.Term]
	interfaceRefs  rows.Pool[keyspace.Term]
	members        rows.Pool[InterfaceMember]
}

// Count reports the sealed row denominator of one declaration family.
func (table Table) Count(family keyspace.Family) int {
	switch family {
	case keyspace.FamilyTypeAlias:
		return table.alias.Count()
	case keyspace.FamilyTypeParam:
		return table.param.Count()
	case keyspace.FamilyTypeInterface:
		return table.iface.Count()
	case keyspace.FamilyDeclaredType:
		return table.declaredType.Count()
	default:
		return 0
	}
}

// TypeParamRow returns the authored type parameter one canonical term names.
// It is the read the TypeParam ownership law uses from its sibling verticals.
func (table Table) TypeParamRow(term keyspace.Term) (TypeParam, bool) {
	return table.param.Row(term)
}

// VisitAliasTypeParams emits every alias-owned type parameter claim against
// the alias that owns it, in canonical alias then source-parameter order.
func (table Table) VisitAliasTypeParams(claim func(owner, param keyspace.Term) bool) bool {
	if claim == nil {
		return false
	}
	for owner, row := range table.alias.Terms() {
		for _, param := range table.aliasParams.All(row.Params) {
			if !claim(owner, param) {
				return false
			}
		}
	}
	return true
}

// VisitInterfaceMethods emits every interface method member against the
// interface that owns it. Signatures proves the member's TypeFunction scope
// is exactly this owner without reaching into declaration storage.
func (table Table) VisitInterfaceMethods(visit func(owner, signature keyspace.Term) bool) bool {
	if visit == nil {
		return false
	}
	for owner, row := range table.iface.Terms() {
		for _, member := range table.members.All(row.Members) {
			if member.Kind != InterfaceMethod {
				continue
			}
			if !visit(owner, member.Signature) {
				return false
			}
		}
	}
	return true
}
