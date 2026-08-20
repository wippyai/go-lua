package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// StaticNodeKind is the closed authored static-type node vocabulary. Its
// ordinal values remain part of the historical Artifact-ID preimage.
type StaticNodeKind uint8

const (
	StaticNodeInvalid StaticNodeKind = iota
	StaticNodePrimitive
	StaticNodeLiteral
	StaticNodeOptional
	StaticNodeUnion
	StaticNodeIntersection
	StaticNodeGeneric
	StaticNodeArray
	StaticNodeMap
	StaticNodeRecord
	StaticNodeReference
	StaticNodeAlias
	StaticNodeTypeParam
	StaticNodeInterface
	StaticNodeTypeFunction
	StaticNodeTypeOf
	StaticNodeKeyOf
	StaticNodeIndex
	StaticNodeConditional
	StaticNodeAssertion
	StaticNodeUnknown
)

func (kind StaticNodeKind) valid() bool {
	return kind != StaticNodeInvalid && kind < StaticNodeUnknown
}

func newStaticNodeChild(parent, child identity.ContentID, position uint32) bool {
	return parent.Available() && child.Available()
}

func childParent(row interface{ Available() bool }) bool {
	return row != nil && row.Available()
}

// StaticTypeNode is the parent row of the canonical authored static graph.
// Variable-width relations are represented only by spans into their own
// typed Program families.  A node does not carry a universal child column:
// every relation has a distinct row type and a distinct dense denominator.
type StaticTypeNode struct {
	id    identity.ContentID
	owner identity.ContentID
	kind  StaticNodeKind

	name        string
	key         keyspace.Key
	exact       keyspace.LiteralValue
	literal     uint8
	bits        uint64
	flag        bool
	resolution  uint8
	assertParam uint32

	declaration          identity.ContentID
	operand              identity.ContentID
	scope                identity.ContentID
	assertionNarrow      identity.ContentID
	assertionCoordinate  [4]uint32
	typeFunctionVariadic identity.ContentID
	returnsKnown         bool

	// Single-child relations remain typed scalar edges on their parent.  They
	// are not packed into a shared child plane.
	optionalInner        identity.ContentID
	genericBase          identity.ContentID
	aliasTarget          identity.ContentID
	arrayElement         identity.ContentID
	mapKey               identity.ContentID
	mapValue             identity.ContentID
	referenceTarget      identity.ContentID
	typeParamConstraint  identity.ContentID
	keyOfChild           identity.ContentID
	indexObject          identity.ContentID
	indexKey             identity.ContentID
	conditionalCheck     identity.ContentID
	conditionalExtends   identity.ContentID
	conditionalThen      identity.ContentID
	conditionalOtherwise identity.ContentID

	segmentCount uint8
	segments     [4]uint32

	unionOffset, unionCount                                         uint32
	intersectionOffset, intersectionCount                           uint32
	genericArgumentOffset, genericArgumentCount                     uint32
	aliasParameterOffset, aliasParameterCount                       uint32
	interfaceExtendOffset, interfaceExtendCount                     uint32
	interfaceMemberOffset, interfaceMemberCount                     uint32
	typeFunctionTypeParameterOffset, typeFunctionTypeParameterCount uint32
	typeFunctionParameterOffset, typeFunctionParameterCount         uint32
	typeFunctionReturnOffset, typeFunctionReturnCount               uint32
	recordFieldOffset, recordFieldCount                             uint32
	referenceSourceKeyOffset, referenceSourceKeyCount               uint32
	referenceCanonicalKeyOffset, referenceCanonicalKeyCount         uint32
}

// StaticTypeNodeSpec is construction data for one parent row.  It is not a
// second retained graph: NewStaticTypeNode copies it into the flat row, while
// all variable-width members are published through the typed families below.
type StaticTypeNodeSpec struct {
	ID, Owner   identity.ContentID
	Kind        StaticNodeKind
	Name        string
	Key         keyspace.Key
	Exact       keyspace.LiteralValue
	Literal     uint8
	Bits        uint64
	Flag        bool
	Resolution  uint8
	AssertParam uint32

	Declaration, Operand, Scope, AssertionNarrow identity.ContentID
	AssertionCoordinate                          [4]uint32
	TypeFunctionVariadic                         identity.ContentID
	ReturnsKnown                                 bool

	OptionalInner, GenericBase, AliasTarget, ArrayElement                       identity.ContentID
	MapKey, MapValue, ReferenceTarget, TypeParamConstraint                      identity.ContentID
	KeyOfChild, IndexObject, IndexKey                                           identity.ContentID
	ConditionalCheck, ConditionalExtends, ConditionalThen, ConditionalOtherwise identity.ContentID

	SegmentCount                                                    uint8
	Segments                                                        [4]uint32
	UnionOffset, UnionCount                                         uint32
	IntersectionOffset, IntersectionCount                           uint32
	GenericArgumentOffset, GenericArgumentCount                     uint32
	AliasParameterOffset, AliasParameterCount                       uint32
	InterfaceExtendOffset, InterfaceExtendCount                     uint32
	InterfaceMemberOffset, InterfaceMemberCount                     uint32
	TypeFunctionTypeParameterOffset, TypeFunctionTypeParameterCount uint32
	TypeFunctionParameterOffset, TypeFunctionParameterCount         uint32
	TypeFunctionReturnOffset, TypeFunctionReturnCount               uint32
	RecordFieldOffset, RecordFieldCount                             uint32
	ReferenceSourceKeyOffset, ReferenceSourceKeyCount               uint32
	ReferenceCanonicalKeyOffset, ReferenceCanonicalKeyCount         uint32
}

// NewStaticTypeNode seals one flat parent row. Child and metadata rows are
// separately constructed and passed to their corresponding Publication
// families; no slice or universal child vocabulary crosses this boundary.
func NewStaticTypeNode(spec StaticTypeNodeSpec) (StaticTypeNode, bool) {
	row := StaticTypeNode{
		id: spec.ID, owner: spec.Owner, kind: spec.Kind, name: spec.Name, key: spec.Key,
		exact: spec.Exact, literal: spec.Literal, bits: spec.Bits, flag: spec.Flag,
		resolution: spec.Resolution, assertParam: spec.AssertParam,
		declaration: spec.Declaration, operand: spec.Operand, scope: spec.Scope,
		assertionNarrow: spec.AssertionNarrow, assertionCoordinate: spec.AssertionCoordinate,
		typeFunctionVariadic: spec.TypeFunctionVariadic, returnsKnown: spec.ReturnsKnown,
		optionalInner: spec.OptionalInner, genericBase: spec.GenericBase, aliasTarget: spec.AliasTarget, arrayElement: spec.ArrayElement,
		mapKey: spec.MapKey, mapValue: spec.MapValue, referenceTarget: spec.ReferenceTarget,
		typeParamConstraint: spec.TypeParamConstraint, keyOfChild: spec.KeyOfChild,
		indexObject: spec.IndexObject, indexKey: spec.IndexKey,
		conditionalCheck: spec.ConditionalCheck, conditionalExtends: spec.ConditionalExtends,
		conditionalThen: spec.ConditionalThen, conditionalOtherwise: spec.ConditionalOtherwise,
		segmentCount: spec.SegmentCount, segments: spec.Segments,
		unionOffset: spec.UnionOffset, unionCount: spec.UnionCount,
		intersectionOffset: spec.IntersectionOffset, intersectionCount: spec.IntersectionCount,
		genericArgumentOffset: spec.GenericArgumentOffset, genericArgumentCount: spec.GenericArgumentCount,
		aliasParameterOffset: spec.AliasParameterOffset, aliasParameterCount: spec.AliasParameterCount,
		interfaceExtendOffset: spec.InterfaceExtendOffset, interfaceExtendCount: spec.InterfaceExtendCount,
		interfaceMemberOffset: spec.InterfaceMemberOffset, interfaceMemberCount: spec.InterfaceMemberCount,
		typeFunctionTypeParameterOffset: spec.TypeFunctionTypeParameterOffset, typeFunctionTypeParameterCount: spec.TypeFunctionTypeParameterCount,
		typeFunctionParameterOffset: spec.TypeFunctionParameterOffset, typeFunctionParameterCount: spec.TypeFunctionParameterCount,
		typeFunctionReturnOffset: spec.TypeFunctionReturnOffset, typeFunctionReturnCount: spec.TypeFunctionReturnCount,
		recordFieldOffset: spec.RecordFieldOffset, recordFieldCount: spec.RecordFieldCount,
		referenceSourceKeyOffset: spec.ReferenceSourceKeyOffset, referenceSourceKeyCount: spec.ReferenceSourceKeyCount,
		referenceCanonicalKeyOffset: spec.ReferenceCanonicalKeyOffset, referenceCanonicalKeyCount: spec.ReferenceCanonicalKeyCount,
	}
	return row, row.Available()
}

func (row StaticTypeNode) Available() bool {
	return row.id.Available() && row.owner.Available() && row.kind.valid() &&
		row.segmentCount <= uint8(len(row.segments))
}
func (row StaticTypeNode) ID() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.id
}
func (row StaticTypeNode) Owner() identity.ContentID {
	if !row.Available() {
		return identity.ContentID{}
	}
	return row.owner
}
func (row StaticTypeNode) Kind() StaticNodeKind {
	if !row.Available() {
		return StaticNodeInvalid
	}
	return row.kind
}
func (row StaticTypeNode) Name() string {
	if !row.Available() {
		return ""
	}
	return row.name
}
func (row StaticTypeNode) Key() keyspace.Key {
	if !row.Available() {
		return 0
	}
	return row.key
}
func (row StaticTypeNode) Exact() keyspace.LiteralValue {
	if !row.Available() {
		return keyspace.LiteralValue{}
	}
	return row.exact
}
func (row StaticTypeNode) LiteralKind() uint8 {
	if !row.Available() {
		return 0
	}
	return row.literal
}
func (row StaticTypeNode) Bits() uint64 {
	if !row.Available() {
		return 0
	}
	return row.bits
}
func (row StaticTypeNode) Flag() bool { return row.Available() && row.flag }
func (row StaticTypeNode) Resolution() uint8 {
	if !row.Available() {
		return 0
	}
	return row.resolution
}
func (row StaticTypeNode) AssertionParam() uint32 {
	if !row.Available() {
		return 0
	}
	return row.assertParam
}
func (row StaticTypeNode) DeclarationOwner() (identity.ContentID, bool) {
	return row.declaration, row.Available() && row.declaration.Available()
}
func (row StaticTypeNode) OperandID() (identity.ContentID, bool) {
	return row.operand, row.Available() && row.operand.Available()
}
func (row StaticTypeNode) ScopeID() (identity.ContentID, bool) {
	return row.scope, row.Available() && row.scope.Available()
}
func (row StaticTypeNode) AssertionNarrowID() (identity.ContentID, bool) {
	return row.assertionNarrow, row.Available() && row.assertionNarrow.Available()
}
func (row StaticTypeNode) AssertionCoordinate() (uint32, uint32, uint32, uint32) {
	return row.assertionCoordinate[0], row.assertionCoordinate[1], row.assertionCoordinate[2], row.assertionCoordinate[3]
}
func (row StaticTypeNode) TypeFunctionVariadic() (identity.ContentID, bool) {
	return row.typeFunctionVariadic, row.Available() && row.typeFunctionVariadic.Available()
}
func (row StaticTypeNode) ReturnsKnown() bool { return row.Available() && row.returnsKnown }

func (row StaticTypeNode) OptionalInner() (identity.ContentID, bool) {
	return row.optionalInner, row.Available() && row.optionalInner.Available()
}
func (row StaticTypeNode) GenericBase() (identity.ContentID, bool) {
	return row.genericBase, row.Available() && row.genericBase.Available()
}
func (row StaticTypeNode) AliasTarget() (identity.ContentID, bool) {
	return row.aliasTarget, row.Available() && row.aliasTarget.Available()
}
func (row StaticTypeNode) ArrayElement() (identity.ContentID, bool) {
	return row.arrayElement, row.Available() && row.arrayElement.Available()
}
func (row StaticTypeNode) MapKey() (identity.ContentID, bool) {
	return row.mapKey, row.Available() && row.mapKey.Available()
}
func (row StaticTypeNode) MapValue() (identity.ContentID, bool) {
	return row.mapValue, row.Available() && row.mapValue.Available()
}
func (row StaticTypeNode) ReferenceTarget() (identity.ContentID, bool) {
	return row.referenceTarget, row.Available() && row.referenceTarget.Available()
}
func (row StaticTypeNode) TypeParamConstraint() (identity.ContentID, bool) {
	return row.typeParamConstraint, row.Available() && row.typeParamConstraint.Available()
}
func (row StaticTypeNode) KeyOfChild() (identity.ContentID, bool) {
	return row.keyOfChild, row.Available() && row.keyOfChild.Available()
}
func (row StaticTypeNode) IndexObject() (identity.ContentID, bool) {
	return row.indexObject, row.Available() && row.indexObject.Available()
}
func (row StaticTypeNode) IndexKey() (identity.ContentID, bool) {
	return row.indexKey, row.Available() && row.indexKey.Available()
}
func (row StaticTypeNode) ConditionalChildren() (identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID, bool) {
	return row.conditionalCheck, row.conditionalExtends, row.conditionalThen, row.conditionalOtherwise,
		row.Available() && row.conditionalCheck.Available() && row.conditionalExtends.Available() && row.conditionalThen.Available() && row.conditionalOtherwise.Available()
}

func (row StaticTypeNode) SegmentCount() int {
	if !row.Available() {
		return 0
	}
	return int(row.segmentCount)
}
func (row StaticTypeNode) SegmentAt(index int) (uint32, bool) {
	if !row.Available() || index < 0 || index >= int(row.segmentCount) {
		return 0, false
	}
	return row.segments[index], true
}

func (row StaticTypeNode) span(offset, count uint32) (uint32, uint32, bool) {
	return offset, count, row.Available() && uint64(offset)+uint64(count) <= uint64(^uint32(0))
}
func (row StaticTypeNode) UnionMemberSpan() (uint32, uint32, bool) {
	return row.span(row.unionOffset, row.unionCount)
}
func (row StaticTypeNode) IntersectionMemberSpan() (uint32, uint32, bool) {
	return row.span(row.intersectionOffset, row.intersectionCount)
}
func (row StaticTypeNode) GenericArgumentSpan() (uint32, uint32, bool) {
	return row.span(row.genericArgumentOffset, row.genericArgumentCount)
}
func (row StaticTypeNode) AliasParameterSpan() (uint32, uint32, bool) {
	return row.span(row.aliasParameterOffset, row.aliasParameterCount)
}
func (row StaticTypeNode) InterfaceExtendSpan() (uint32, uint32, bool) {
	return row.span(row.interfaceExtendOffset, row.interfaceExtendCount)
}
func (row StaticTypeNode) InterfaceMemberSpan() (uint32, uint32, bool) {
	return row.span(row.interfaceMemberOffset, row.interfaceMemberCount)
}
func (row StaticTypeNode) TypeFunctionTypeParameterSpan() (uint32, uint32, bool) {
	return row.span(row.typeFunctionTypeParameterOffset, row.typeFunctionTypeParameterCount)
}
func (row StaticTypeNode) TypeFunctionParameterSpan() (uint32, uint32, bool) {
	return row.span(row.typeFunctionParameterOffset, row.typeFunctionParameterCount)
}
func (row StaticTypeNode) TypeFunctionReturnSpan() (uint32, uint32, bool) {
	return row.span(row.typeFunctionReturnOffset, row.typeFunctionReturnCount)
}
func (row StaticTypeNode) RecordFieldSpan() (uint32, uint32, bool) {
	return row.span(row.recordFieldOffset, row.recordFieldCount)
}
func (row StaticTypeNode) ReferenceSourceKeySpan() (uint32, uint32, bool) {
	return row.span(row.referenceSourceKeyOffset, row.referenceSourceKeyCount)
}
func (row StaticTypeNode) ReferenceCanonicalKeySpan() (uint32, uint32, bool) {
	return row.span(row.referenceCanonicalKeyOffset, row.referenceCanonicalKeyCount)
}
