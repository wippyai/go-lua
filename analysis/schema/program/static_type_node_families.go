package programschema

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
)

// Append-only slots keep all pre-existing Program family addresses stable.
// Static type rows were added after the diagnostic catalog, so existing
// local-transfer, occurrence, and diagnostic addresses remain unchanged.
const (
	slotStaticTypeNode                          = slotDiagnosticPath + 1
	slotStaticTypeNodeUnionMember               = slotStaticTypeNode + 1
	slotStaticTypeNodeIntersectionMember        = slotStaticTypeNodeUnionMember + 1
	slotStaticTypeNodeGenericArgument           = slotStaticTypeNodeIntersectionMember + 1
	slotStaticTypeNodeAliasParameter            = slotStaticTypeNodeGenericArgument + 1
	slotStaticTypeNodeInterfaceExtend           = slotStaticTypeNodeAliasParameter + 1
	slotStaticTypeNodeInterfaceMember           = slotStaticTypeNodeInterfaceExtend + 1
	slotStaticTypeNodeTypeFunctionTypeParameter = slotStaticTypeNodeInterfaceMember + 1
	slotStaticTypeNodeTypeFunctionParameter     = slotStaticTypeNodeTypeFunctionTypeParameter + 1
	slotStaticTypeNodeTypeFunctionReturn        = slotStaticTypeNodeTypeFunctionParameter + 1
	slotStaticTypeNodeRecordField               = slotStaticTypeNodeTypeFunctionReturn + 1
	slotStaticTypeNodeReferenceSourceKey        = slotStaticTypeNodeRecordField + 1
	slotStaticTypeNodeReferenceCanonicalKey     = slotStaticTypeNodeReferenceSourceKey + 1
)

var (
	staticTypeNodeFamily                          = Family[StaticTypeNode]{slot: slotStaticTypeNode, name: "static-type-node"}
	staticTypeNodeUnionMemberFamily               = Family[StaticTypeNodeUnionMember]{slot: slotStaticTypeNodeUnionMember, name: "static-type-node-union-member"}
	staticTypeNodeIntersectionMemberFamily        = Family[StaticTypeNodeIntersectionMember]{slot: slotStaticTypeNodeIntersectionMember, name: "static-type-node-intersection-member"}
	staticTypeNodeGenericArgumentFamily           = Family[StaticTypeNodeGenericArgument]{slot: slotStaticTypeNodeGenericArgument, name: "static-type-node-generic-argument"}
	staticTypeNodeAliasParameterFamily            = Family[StaticTypeNodeAliasParameter]{slot: slotStaticTypeNodeAliasParameter, name: "static-type-node-alias-parameter"}
	staticTypeNodeInterfaceExtendFamily           = Family[StaticTypeNodeInterfaceExtend]{slot: slotStaticTypeNodeInterfaceExtend, name: "static-type-node-interface-extend"}
	staticTypeNodeInterfaceMemberFamily           = Family[StaticTypeNodeInterfaceMember]{slot: slotStaticTypeNodeInterfaceMember, name: "static-type-node-interface-member"}
	staticTypeNodeTypeFunctionTypeParameterFamily = Family[StaticTypeNodeTypeFunctionTypeParameter]{slot: slotStaticTypeNodeTypeFunctionTypeParameter, name: "static-type-node-type-function-type-parameter"}
	staticTypeNodeTypeFunctionParameterFamily     = Family[StaticTypeNodeTypeFunctionParameter]{slot: slotStaticTypeNodeTypeFunctionParameter, name: "static-type-node-type-function-parameter"}
	staticTypeNodeTypeFunctionReturnFamily        = Family[StaticTypeNodeTypeFunctionReturn]{slot: slotStaticTypeNodeTypeFunctionReturn, name: "static-type-node-type-function-return"}
	staticTypeNodeRecordFieldFamily               = Family[StaticTypeNodeRecordField]{slot: slotStaticTypeNodeRecordField, name: "static-type-node-record-field"}
	staticTypeNodeReferenceSourceKeyFamily        = Family[StaticTypeNodeReferenceSourceKey]{slot: slotStaticTypeNodeReferenceSourceKey, name: "static-type-node-reference-source-key"}
	staticTypeNodeReferenceCanonicalKeyFamily     = Family[StaticTypeNodeReferenceCanonicalKey]{slot: slotStaticTypeNodeReferenceCanonicalKey, name: "static-type-node-reference-canonical-key"}
)

func StaticTypeNodeFamily() Family[StaticTypeNode] { return staticTypeNodeFamily }
func StaticTypeNodeUnionMemberFamily() Family[StaticTypeNodeUnionMember] {
	return staticTypeNodeUnionMemberFamily
}
func StaticTypeNodeIntersectionMemberFamily() Family[StaticTypeNodeIntersectionMember] {
	return staticTypeNodeIntersectionMemberFamily
}
func StaticTypeNodeGenericArgumentFamily() Family[StaticTypeNodeGenericArgument] {
	return staticTypeNodeGenericArgumentFamily
}
func StaticTypeNodeAliasParameterFamily() Family[StaticTypeNodeAliasParameter] {
	return staticTypeNodeAliasParameterFamily
}
func StaticTypeNodeInterfaceExtendFamily() Family[StaticTypeNodeInterfaceExtend] {
	return staticTypeNodeInterfaceExtendFamily
}
func StaticTypeNodeInterfaceMemberFamily() Family[StaticTypeNodeInterfaceMember] {
	return staticTypeNodeInterfaceMemberFamily
}
func StaticTypeNodeTypeFunctionTypeParameterFamily() Family[StaticTypeNodeTypeFunctionTypeParameter] {
	return staticTypeNodeTypeFunctionTypeParameterFamily
}
func StaticTypeNodeTypeFunctionParameterFamily() Family[StaticTypeNodeTypeFunctionParameter] {
	return staticTypeNodeTypeFunctionParameterFamily
}
func StaticTypeNodeTypeFunctionReturnFamily() Family[StaticTypeNodeTypeFunctionReturn] {
	return staticTypeNodeTypeFunctionReturnFamily
}
func StaticTypeNodeRecordFieldFamily() Family[StaticTypeNodeRecordField] {
	return staticTypeNodeRecordFieldFamily
}
func StaticTypeNodeReferenceSourceKeyFamily() Family[StaticTypeNodeReferenceSourceKey] {
	return staticTypeNodeReferenceSourceKeyFamily
}
func StaticTypeNodeReferenceCanonicalKeyFamily() Family[StaticTypeNodeReferenceCanonicalKey] {
	return staticTypeNodeReferenceCanonicalKeyFamily
}

func staticTypeNodeFamilyCount[V Row](row Program, family Family[V]) (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	return family.Count(&row.Frozen, catalog)
}

func staticTypeNodeFamilyAt[V Row](row Program, family Family[V], index int) (V, bool) {
	var absent V
	catalog, ok := row.catalog()
	if !ok {
		return absent, false
	}
	return family.At(&row.Frozen, catalog, index)
}

func (row Program) StaticTypeNodeCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeFamily())
}
func (row Program) StaticTypeNodeAt(index int) (StaticTypeNode, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeFamily(), index)
}
func (row Program) StaticTypeNodeUnionMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeUnionMemberFamily())
}
func (row Program) StaticTypeNodeUnionMemberAt(index int) (StaticTypeNodeUnionMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeUnionMemberFamily(), index)
}
func (row Program) StaticTypeNodeIntersectionMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeIntersectionMemberFamily())
}
func (row Program) StaticTypeNodeIntersectionMemberAt(index int) (StaticTypeNodeIntersectionMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeIntersectionMemberFamily(), index)
}
func (row Program) StaticTypeNodeGenericArgumentCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeGenericArgumentFamily())
}
func (row Program) StaticTypeNodeGenericArgumentAt(index int) (StaticTypeNodeGenericArgument, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeGenericArgumentFamily(), index)
}
func (row Program) StaticTypeNodeAliasParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeAliasParameterFamily())
}
func (row Program) StaticTypeNodeAliasParameterAt(index int) (StaticTypeNodeAliasParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeAliasParameterFamily(), index)
}
func (row Program) StaticTypeNodeInterfaceExtendCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeInterfaceExtendFamily())
}
func (row Program) StaticTypeNodeInterfaceExtendAt(index int) (StaticTypeNodeInterfaceExtend, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeInterfaceExtendFamily(), index)
}
func (row Program) StaticTypeNodeInterfaceMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeInterfaceMemberFamily())
}
func (row Program) StaticTypeNodeInterfaceMemberAt(index int) (StaticTypeNodeInterfaceMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeInterfaceMemberFamily(), index)
}
func (row Program) StaticTypeNodeTypeFunctionTypeParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionTypeParameterFamily())
}
func (row Program) StaticTypeNodeTypeFunctionTypeParameterAt(index int) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionTypeParameterFamily(), index)
}
func (row Program) StaticTypeNodeTypeFunctionParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionParameterFamily())
}
func (row Program) StaticTypeNodeTypeFunctionParameterAt(index int) (StaticTypeNodeTypeFunctionParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionParameterFamily(), index)
}
func (row Program) StaticTypeNodeTypeFunctionReturnCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionReturnFamily())
}
func (row Program) StaticTypeNodeTypeFunctionReturnAt(index int) (StaticTypeNodeTypeFunctionReturn, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionReturnFamily(), index)
}
func (row Program) StaticTypeNodeRecordFieldCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeRecordFieldFamily())
}
func (row Program) StaticTypeNodeRecordFieldAt(index int) (StaticTypeNodeRecordField, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeRecordFieldFamily(), index)
}
func (row Program) StaticTypeNodeReferenceSourceKeyCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeReferenceSourceKeyFamily())
}
func (row Program) StaticTypeNodeReferenceSourceKeyAt(index int) (StaticTypeNodeReferenceSourceKey, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeReferenceSourceKeyFamily(), index)
}
func (row Program) StaticTypeNodeReferenceCanonicalKeyCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeReferenceCanonicalKeyFamily())
}
func (row Program) StaticTypeNodeReferenceCanonicalKeyAt(index int) (StaticTypeNodeReferenceCanonicalKey, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeReferenceCanonicalKeyFamily(), index)
}

type staticTypeNodeChildRow interface {
	Row
	ChildID() identity.ContentID
}

func staticTypeNodeProgramCatalog(row Program) (identity.ContentID, bool) {
	if !row.Frozen.Published() || !row.ProgramID.Available() || !row.SchemaID.Available() {
		return identity.ContentID{}, false
	}
	return CatalogID(row.SchemaID)
}

func staticTypeNodeFamilyAtSealed[V Row](row Program, family Family[V], catalog identity.ContentID, index int) (V, bool) {
	var absent V
	if !catalog.Available() {
		return absent, false
	}
	return family.At(&row.Frozen, catalog, index)
}

func staticTypeNodeFamilyForSealed[V staticTypeNodeParentRow](row Program, index, childIndex int, family Family[V], span func(StaticTypeNode) (uint32, uint32, bool)) (V, bool) {
	var absent V
	catalog, catalogOK := staticTypeNodeProgramCatalog(row)
	if !catalogOK || index < 0 || childIndex < 0 {
		return absent, false
	}
	parent, parentOK := staticTypeNodeFamilyAtSealed(row, StaticTypeNodeFamily(), catalog, index)
	if !parentOK {
		return absent, false
	}
	offset, count, spanOK := span(parent)
	if !spanOK || uint64(childIndex) >= uint64(count) {
		return absent, false
	}
	child, childOK := staticTypeNodeFamilyAtSealed(row, family, catalog, int(offset)+childIndex)
	return child, childOK && child.ParentID() == parent.ID() && child.Position() == uint32(childIndex)
}

func appendStaticTypeNodeFamily[V staticTypeNodeChildRow](count uint32, at func(int) (V, bool), children *[]identity.ContentID) bool {
	for position := uint32(0); position < count; position++ {
		child, ok := at(int(position))
		if !ok || !child.Available() || !child.ChildID().Available() {
			return false
		}
		*children = append(*children, child.ChildID())
	}
	return true
}

func staticTypeNodeMetadataSpan[V Row](parent identity.ContentID, offset, count uint32, at func(int) (V, bool), parentID func(V) identity.ContentID, position func(V) uint32) bool {
	for ordinal := uint32(0); ordinal < count; ordinal++ {
		row, ok := at(int(offset + ordinal))
		if !ok || !row.Available() || parentID(row) != parent || position(row) != ordinal {
			return false
		}
	}
	return true
}

// StaticTypeNodeChildren reconstructs the semantic child order of one
// canonical static node from its typed Program families. The order is shared
// by Artifact identity replay and the detached type authority; no generic
// child row or projected graph is retained. Strict reference validation is
// used by Artifact sealing, while identity replay accepts the historical
// target-optional form for unresolved references.
func (program Program) StaticTypeNodeChildren(index int, row StaticTypeNode, strict bool) ([]identity.ContentID, bool) {
	catalog, catalogOK := staticTypeNodeProgramCatalog(program)
	if !catalogOK || index < 0 {
		return nil, false
	}
	owner, ownerOK := staticTypeNodeFamilyAtSealed(program, StaticTypeNodeFamily(), catalog, index)
	if !ownerOK || owner.ID() != row.ID() {
		return nil, false
	}
	var result []identity.ContentID
	add := func(id identity.ContentID, ok bool) bool {
		if !ok || !id.Available() {
			return false
		}
		result = append(result, id)
		return true
	}
	optional := func(id identity.ContentID, present bool) bool {
		if !present {
			return true
		}
		return add(id, true)
	}
	switch row.Kind() {
	case StaticNodeOptional:
		id, ok := row.OptionalInner()
		if !add(id, ok) {
			return nil, false
		}
	case StaticNodeUnion:
		_, count, spanOK := row.UnionMemberSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeUnionMember, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeUnionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.UnionMemberSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeIntersection:
		_, count, spanOK := row.IntersectionMemberSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeIntersectionMember, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeIntersectionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.IntersectionMemberSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeGeneric:
		id, ok := row.GenericBase()
		if !add(id, ok) {
			return nil, false
		}
		_, count, spanOK := row.GenericArgumentSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeGenericArgument, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeGenericArgumentFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.GenericArgumentSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeArray:
		id, ok := row.ArrayElement()
		if !add(id, ok) {
			return nil, false
		}
	case StaticNodeMap:
		id, ok := row.MapKey()
		if !add(id, ok) {
			return nil, false
		}
		id, ok = row.MapValue()
		if !add(id, ok) {
			return nil, false
		}
	case StaticNodeRecord:
		_, count, spanOK := row.RecordFieldSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeRecordField, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeRecordFieldFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.RecordFieldSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeReference:
		id, ok := row.ReferenceTarget()
		if strict {
			switch staticrefs.Resolution(row.Resolution()) {
			case staticrefs.Unresolved:
				if id.Available() {
					return nil, false
				}
			case staticrefs.Declaration, staticrefs.CanonicalPath:
				if !add(id, ok) {
					return nil, false
				}
			default:
				return nil, false
			}
		} else if id.Available() && !optional(id, ok) {
			return nil, false
		}
		sourceOffset, sourceCount, sourceOK := row.ReferenceSourceKeySpan()
		canonicalOffset, canonicalCount, canonicalOK := row.ReferenceCanonicalKeySpan()
		if !sourceOK || !canonicalOK ||
			!staticTypeNodeMetadataSpan(row.ID(), sourceOffset, sourceCount, func(index int) (StaticTypeNodeReferenceSourceKey, bool) {
				return staticTypeNodeFamilyAtSealed(program, StaticTypeNodeReferenceSourceKeyFamily(), catalog, index)
			},
				func(value StaticTypeNodeReferenceSourceKey) identity.ContentID { return value.ParentID() },
				func(value StaticTypeNodeReferenceSourceKey) uint32 { return value.Position() }) ||
			!staticTypeNodeMetadataSpan(row.ID(), canonicalOffset, canonicalCount, func(index int) (StaticTypeNodeReferenceCanonicalKey, bool) {
				return staticTypeNodeFamilyAtSealed(program, StaticTypeNodeReferenceCanonicalKeyFamily(), catalog, index)
			},
				func(value StaticTypeNodeReferenceCanonicalKey) identity.ContentID { return value.ParentID() },
				func(value StaticTypeNodeReferenceCanonicalKey) uint32 { return value.Position() }) {
			return nil, false
		}
	case StaticNodeAlias:
		id, ok := row.AliasTarget()
		if !add(id, ok) {
			return nil, false
		}
		_, count, spanOK := row.AliasParameterSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeAliasParameter, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeAliasParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.AliasParameterSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeTypeParam:
		id, ok := row.TypeParamConstraint()
		if id.Available() && !optional(id, ok) {
			return nil, false
		}
	case StaticNodeInterface:
		_, count, spanOK := row.InterfaceExtendSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeInterfaceExtend, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeInterfaceExtendFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceExtendSpan() })
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.InterfaceMemberSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeInterfaceMember, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeInterfaceMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceMemberSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeTypeFunction:
		if id, ok := row.TypeFunctionVariadic(); ok && !add(id, true) {
			return nil, false
		}
		_, count, spanOK := row.TypeFunctionTypeParameterSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeTypeFunctionTypeParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionTypeParameterSpan() })
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.TypeFunctionParameterSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeTypeFunctionParameter, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeTypeFunctionParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionParameterSpan() })
		}, &result) {
			return nil, false
		}
		_, count, spanOK = row.TypeFunctionReturnSpan()
		if !spanOK || !appendStaticTypeNodeFamily(count, func(position int) (StaticTypeNodeTypeFunctionReturn, bool) {
			return staticTypeNodeFamilyForSealed(program, index, position, StaticTypeNodeTypeFunctionReturnFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionReturnSpan() })
		}, &result) {
			return nil, false
		}
	case StaticNodeKeyOf:
		id, ok := row.KeyOfChild()
		if !add(id, ok) {
			return nil, false
		}
	case StaticNodeIndex:
		id, ok := row.IndexObject()
		if !add(id, ok) {
			return nil, false
		}
		id, ok = row.IndexKey()
		if !add(id, ok) {
			return nil, false
		}
	case StaticNodeConditional:
		a, b, c, d, ok := row.ConditionalChildren()
		if !ok || !add(a, true) || !add(b, true) || !add(c, true) || !add(d, true) {
			return nil, false
		}
	case StaticNodeAssertion:
		id, ok := row.AssertionNarrowID()
		if id.Available() && !optional(id, ok) {
			return nil, false
		}
	}
	return result, true
}

type staticTypeNodeParentRow interface {
	Row
	ParentID() identity.ContentID
	Position() uint32
}

func staticTypeNodeFamilyFor[V staticTypeNodeParentRow](row Program, nodeIndex, childIndex int, family Family[V], span func(StaticTypeNode) (uint32, uint32, bool)) (V, bool) {
	var absent V
	parent, ok := row.StaticTypeNodeAt(nodeIndex)
	if !ok {
		return absent, false
	}
	offset, count, ok := span(parent)
	if !ok || childIndex < 0 || uint64(childIndex) >= uint64(count) {
		return absent, false
	}
	child, ok := familyAtProgram(row, family, int(offset)+childIndex)
	return child, ok && child.ParentID() == parent.ID() && child.Position() == uint32(childIndex)
}

func familyAtProgram[V Row](row Program, family Family[V], index int) (V, bool) {
	return staticTypeNodeFamilyAt(row, family, index)
}

func (row Program) StaticTypeNodeUnionMemberFor(nodeIndex, childIndex int) (StaticTypeNodeUnionMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeUnionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.UnionMemberSpan() })
}
func (row Program) StaticTypeNodeIntersectionMemberFor(nodeIndex, childIndex int) (StaticTypeNodeIntersectionMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeIntersectionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.IntersectionMemberSpan() })
}
func (row Program) StaticTypeNodeGenericArgumentFor(nodeIndex, childIndex int) (StaticTypeNodeGenericArgument, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeGenericArgumentFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.GenericArgumentSpan() })
}
func (row Program) StaticTypeNodeAliasParameterFor(nodeIndex, childIndex int) (StaticTypeNodeAliasParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeAliasParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.AliasParameterSpan() })
}
func (row Program) StaticTypeNodeInterfaceExtendFor(nodeIndex, childIndex int) (StaticTypeNodeInterfaceExtend, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeInterfaceExtendFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceExtendSpan() })
}
func (row Program) StaticTypeNodeInterfaceMemberFor(nodeIndex, childIndex int) (StaticTypeNodeInterfaceMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeInterfaceMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceMemberSpan() })
}
func (row Program) StaticTypeNodeTypeFunctionTypeParameterFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionTypeParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionTypeParameterSpan() })
}
func (row Program) StaticTypeNodeTypeFunctionParameterFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionParameterSpan() })
}
func (row Program) StaticTypeNodeTypeFunctionReturnFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionReturn, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionReturnFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionReturnSpan() })
}
func (row Program) StaticTypeNodeRecordFieldFor(nodeIndex, childIndex int) (StaticTypeNodeRecordField, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeRecordFieldFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.RecordFieldSpan() })
}
func (row Program) StaticTypeNodeReferenceSourceKeyFor(nodeIndex, childIndex int) (StaticTypeNodeReferenceSourceKey, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeReferenceSourceKeyFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.ReferenceSourceKeySpan() })
}
func (row Program) StaticTypeNodeReferenceCanonicalKeyFor(nodeIndex, childIndex int) (StaticTypeNodeReferenceCanonicalKey, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeReferenceCanonicalKeyFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.ReferenceCanonicalKeySpan() })
}
