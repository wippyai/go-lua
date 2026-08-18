package static

import (
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// TypeAlias, TypeParam, Interface, and InterfaceMember are distinct authored
// declaration relations. They deliberately do not form a generic declaration
// node/edge vocabulary: every ownership and ordering law remains visible here.
type TypeAlias struct {
	Owner          keyspace.Term
	Target         keyspace.Term
	Name           keyspace.Key
	NameCoordinate source.Coordinate
	Params         []keyspace.Term
}

// TypeParam has no secondary coordinate: the owning Term span is its name.
// Its owner may be a TypeAlias, a later Signature TypeFunction, or Flow's
// authored Function family.
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

type DeclarationsInput struct {
	Alias        []TypeAlias
	TypeParam    []TypeParam
	Interface    []Interface
	DeclaredType []DeclaredType
}

type declarationStore struct {
	aliases        []typeAliasRow
	params         []TypeParam
	interfaces     []interfaceRow
	declaredTypes  []declaredTypeRow
	declaredByCell []keyspace.Term
	aliasParams    []keyspace.Term
	interfaceRefs  []keyspace.Term
	members        []interfaceMemberRow
}

type declaredTypeRow struct {
	cell   keyspace.Term
	target keyspace.Term
}

type typeAliasRow struct {
	owner      keyspace.Term
	target     keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	params     poolRange
}

type interfaceRow struct {
	owner      keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	extends    poolRange
	members    poolRange
}

type interfaceMemberRow struct {
	kind       InterfaceMemberKind
	field      keyspace.Term
	name       keyspace.Key
	coordinate source.Coordinate
	signature  keyspace.Term
}

type Declarations struct {
	component *Component
	state     *draftState
}
type Aliases struct {
	component *Component
	state     *draftState
}
type TypeParams struct {
	component *Component
	state     *draftState
}
type Interfaces struct {
	component *Component
	state     *draftState
}
type DeclaredTypes struct {
	component *Component
	state     *draftState
}
