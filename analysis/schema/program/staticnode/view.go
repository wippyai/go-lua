package staticnode

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	programstate "github.com/wippyai/go-lua/analysis/schema/program/state"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// View is the read capability for one authenticated cold Program publication.
// It retains only the immutable program state; rows, indexes, and Program
// metadata remain outside this package.
type View struct {
	state programstate.State
}

// NewView authenticates a static-node reader over one sealed program state.
func NewView(state programstate.State) (View, bool) {
	if !state.Available() {
		return View{}, false
	}
	return View{state: state}, true
}

// Available reports whether this reader has an authenticated sealed state.
func (view View) Available() bool { return view.state.Available() }

// State exposes the immutable state capability used by this view.
func (view View) State() programstate.State { return view.state }

func (view View) catalog() (identity.ContentID, bool) {
	if !view.Available() {
		return identity.ContentID{}, false
	}
	return view.state.CatalogID(), true
}

func (view View) frozen() snapshot.Frozen { return view.state.Frozen() }

func staticTypeNodeFamilyCount[V programfamily.Row](row View, family programfamily.Family[V]) (int, bool) {
	catalog, ok := row.catalog()
	if !ok {
		return 0, false
	}
	frozen := row.frozen()
	return family.Count(&frozen, catalog)
}

func staticTypeNodeFamilyAt[V programfamily.Row](row View, family programfamily.Family[V], index int) (V, bool) {
	var absent V
	catalog, ok := row.catalog()
	if !ok {
		return absent, false
	}
	frozen := row.frozen()
	return family.At(&frozen, catalog, index)
}

func (row View) StaticTypeNodeCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeFamily())
}
func (row View) StaticTypeNodeAt(index int) (StaticTypeNode, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeFamily(), index)
}
func (row View) StaticTypeNodeUnionMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeUnionMemberFamily())
}
func (row View) StaticTypeNodeUnionMemberAt(index int) (StaticTypeNodeUnionMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeUnionMemberFamily(), index)
}
func (row View) StaticTypeNodeIntersectionMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeIntersectionMemberFamily())
}
func (row View) StaticTypeNodeIntersectionMemberAt(index int) (StaticTypeNodeIntersectionMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeIntersectionMemberFamily(), index)
}
func (row View) StaticTypeNodeGenericArgumentCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeGenericArgumentFamily())
}
func (row View) StaticTypeNodeGenericArgumentAt(index int) (StaticTypeNodeGenericArgument, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeGenericArgumentFamily(), index)
}
func (row View) StaticTypeNodeAliasParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeAliasParameterFamily())
}
func (row View) StaticTypeNodeAliasParameterAt(index int) (StaticTypeNodeAliasParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeAliasParameterFamily(), index)
}
func (row View) StaticTypeNodeInterfaceExtendCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeInterfaceExtendFamily())
}
func (row View) StaticTypeNodeInterfaceExtendAt(index int) (StaticTypeNodeInterfaceExtend, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeInterfaceExtendFamily(), index)
}
func (row View) StaticTypeNodeInterfaceMemberCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeInterfaceMemberFamily())
}
func (row View) StaticTypeNodeInterfaceMemberAt(index int) (StaticTypeNodeInterfaceMember, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeInterfaceMemberFamily(), index)
}
func (row View) StaticTypeNodeTypeFunctionTypeParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionTypeParameterFamily())
}
func (row View) StaticTypeNodeTypeFunctionTypeParameterAt(index int) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionTypeParameterFamily(), index)
}
func (row View) StaticTypeNodeTypeFunctionParameterCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionParameterFamily())
}
func (row View) StaticTypeNodeTypeFunctionParameterAt(index int) (StaticTypeNodeTypeFunctionParameter, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionParameterFamily(), index)
}
func (row View) StaticTypeNodeTypeFunctionReturnCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeTypeFunctionReturnFamily())
}
func (row View) StaticTypeNodeTypeFunctionReturnAt(index int) (StaticTypeNodeTypeFunctionReturn, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeTypeFunctionReturnFamily(), index)
}
func (row View) StaticTypeNodeRecordFieldCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeRecordFieldFamily())
}
func (row View) StaticTypeNodeRecordFieldAt(index int) (StaticTypeNodeRecordField, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeRecordFieldFamily(), index)
}
func (row View) StaticTypeNodeReferenceSourceKeyCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeReferenceSourceKeyFamily())
}
func (row View) StaticTypeNodeReferenceSourceKeyAt(index int) (StaticTypeNodeReferenceSourceKey, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeReferenceSourceKeyFamily(), index)
}
func (row View) StaticTypeNodeReferenceCanonicalKeyCount() (int, bool) {
	return staticTypeNodeFamilyCount(row, StaticTypeNodeReferenceCanonicalKeyFamily())
}
func (row View) StaticTypeNodeReferenceCanonicalKeyAt(index int) (StaticTypeNodeReferenceCanonicalKey, bool) {
	return staticTypeNodeFamilyAt(row, StaticTypeNodeReferenceCanonicalKeyFamily(), index)
}

type staticTypeNodeChildRow interface {
	programfamily.Row
	ChildID() identity.ContentID
}

func staticTypeNodeViewCatalog(row View) (identity.ContentID, bool) {
	return row.catalog()
}

func staticTypeNodeFamilyAtSealed[V programfamily.Row](row View, family programfamily.Family[V], catalog identity.ContentID, index int) (V, bool) {
	var absent V
	if !catalog.Available() {
		return absent, false
	}
	frozen := row.frozen()
	return family.At(&frozen, catalog, index)
}

func staticTypeNodeFamilyForSealed[V staticTypeNodeParentRow](row View, index, childIndex int, family programfamily.Family[V], span func(StaticTypeNode) (uint32, uint32, bool)) (V, bool) {
	var absent V
	catalog, catalogOK := staticTypeNodeViewCatalog(row)
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

func staticTypeNodeMetadataSpan[V programfamily.Row](parent identity.ContentID, offset, count uint32, at func(int) (V, bool), parentID func(V) identity.ContentID, position func(V) uint32) bool {
	for ordinal := uint32(0); ordinal < count; ordinal++ {
		row, ok := at(int(offset + ordinal))
		if !ok || !row.Available() || parentID(row) != parent || position(row) != ordinal {
			return false
		}
	}
	return true
}

// StaticTypeNodeChildren reconstructs the semantic child order of one
// canonical static node from its typed View families. The order is shared
// by Artifact identity replay and the detached type authority; no generic
// child row or projected graph is retained. Strict reference validation is
// used by Artifact sealing, while identity replay accepts the historical
// target-optional form for unresolved references.
func (program View) StaticTypeNodeChildren(index int, row StaticTypeNode, strict bool) ([]identity.ContentID, bool) {
	catalog, catalogOK := staticTypeNodeViewCatalog(program)
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
	programfamily.Row
	ParentID() identity.ContentID
	Position() uint32
}

func staticTypeNodeFamilyFor[V staticTypeNodeParentRow](row View, nodeIndex, childIndex int, family programfamily.Family[V], span func(StaticTypeNode) (uint32, uint32, bool)) (V, bool) {
	var absent V
	parent, ok := row.StaticTypeNodeAt(nodeIndex)
	if !ok {
		return absent, false
	}
	offset, count, ok := span(parent)
	if !ok || childIndex < 0 || uint64(childIndex) >= uint64(count) {
		return absent, false
	}
	child, ok := familyAtView(row, family, int(offset)+childIndex)
	return child, ok && child.ParentID() == parent.ID() && child.Position() == uint32(childIndex)
}

func familyAtView[V programfamily.Row](row View, family programfamily.Family[V], index int) (V, bool) {
	return staticTypeNodeFamilyAt(row, family, index)
}

func (row View) StaticTypeNodeUnionMemberFor(nodeIndex, childIndex int) (StaticTypeNodeUnionMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeUnionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.UnionMemberSpan() })
}
func (row View) StaticTypeNodeIntersectionMemberFor(nodeIndex, childIndex int) (StaticTypeNodeIntersectionMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeIntersectionMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.IntersectionMemberSpan() })
}
func (row View) StaticTypeNodeGenericArgumentFor(nodeIndex, childIndex int) (StaticTypeNodeGenericArgument, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeGenericArgumentFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.GenericArgumentSpan() })
}
func (row View) StaticTypeNodeAliasParameterFor(nodeIndex, childIndex int) (StaticTypeNodeAliasParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeAliasParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.AliasParameterSpan() })
}
func (row View) StaticTypeNodeInterfaceExtendFor(nodeIndex, childIndex int) (StaticTypeNodeInterfaceExtend, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeInterfaceExtendFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceExtendSpan() })
}
func (row View) StaticTypeNodeInterfaceMemberFor(nodeIndex, childIndex int) (StaticTypeNodeInterfaceMember, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeInterfaceMemberFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.InterfaceMemberSpan() })
}
func (row View) StaticTypeNodeTypeFunctionTypeParameterFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionTypeParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionTypeParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionTypeParameterSpan() })
}
func (row View) StaticTypeNodeTypeFunctionParameterFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionParameter, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionParameterFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionParameterSpan() })
}
func (row View) StaticTypeNodeTypeFunctionReturnFor(nodeIndex, childIndex int) (StaticTypeNodeTypeFunctionReturn, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeTypeFunctionReturnFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.TypeFunctionReturnSpan() })
}
func (row View) StaticTypeNodeRecordFieldFor(nodeIndex, childIndex int) (StaticTypeNodeRecordField, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeRecordFieldFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.RecordFieldSpan() })
}
func (row View) StaticTypeNodeReferenceSourceKeyFor(nodeIndex, childIndex int) (StaticTypeNodeReferenceSourceKey, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeReferenceSourceKeyFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.ReferenceSourceKeySpan() })
}
func (row View) StaticTypeNodeReferenceCanonicalKeyFor(nodeIndex, childIndex int) (StaticTypeNodeReferenceCanonicalKey, bool) {
	return staticTypeNodeFamilyFor(row, nodeIndex, childIndex, StaticTypeNodeReferenceCanonicalKeyFamily(), func(parent StaticTypeNode) (uint32, uint32, bool) { return parent.ReferenceCanonicalKeySpan() })
}
