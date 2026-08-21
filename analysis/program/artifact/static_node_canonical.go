package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	staticrefs "github.com/wippyai/go-lua/analysis/program/static/references"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
)

// staticNodeFamilySpan is a transient join from a parent span to its typed
// canonical family. The returned slice is resolve-time scratch space; no
// family projection is retained by Artifact.
type staticNodeParentRow interface {
	programschema.Row
	ParentID() identity.ContentID
	Position() uint32
}

func staticNodeFamilySpan[V staticNodeParentRow](artifact *Artifact, parent identity.ContentID, offset, count uint32, family programschema.Family[V]) ([]V, bool) {
	if artifact == nil {
		return nil, false
	}
	if uint64(offset)+uint64(count) > uint64(^uint32(0)) || uint64(count) > uint64(^uint(0)>>1) {
		return nil, false
	}
	rows := make([]V, 0, int(count))
	for position := uint32(0); position < count; position++ {
		row, ok := coldRow(artifact, family, int(offset+position))
		if !ok || !row.Available() {
			return nil, false
		}
		if row.ParentID() != parent || row.Position() != position {
			return nil, false
		}
		rows = append(rows, row)
	}
	return rows, true
}

func staticNodeMetadataSpanSealed[V staticNodeParentRow](artifact *Artifact, parent identity.ContentID, offset, count uint32, family programschema.Family[V]) bool {
	_, ok := staticNodeFamilySpan(artifact, parent, offset, count, family)
	return ok
}

// canonicalStaticNodeChildren reconstructs only the historical semantic child
// order needed by Artifact-ID and arithmetic validation. It is never stored.
func (artifact *Artifact) canonicalStaticNodeChildren(row programschema.StaticTypeNode, strict bool) ([]identity.ContentID, bool) {
	var result []identity.ContentID
	add := func(id identity.ContentID, present bool) bool {
		if !present || !id.Available() {
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
	span := func(offset, count uint32, addRow func(uint32) (identity.ContentID, bool)) bool {
		for position := uint32(0); position < count; position++ {
			id, ok := addRow(offset + position)
			if !add(id, ok) {
				return false
			}
		}
		return true
	}
	switch row.Kind() {
	case programschema.StaticNodeOptional:
		id, ok := row.OptionalInner()
		return result, add(id, ok)
	case programschema.StaticNodeUnion:
		offset, count, ok := row.UnionMemberSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeUnionMemberFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeIntersection:
		offset, count, ok := row.IntersectionMemberSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeIntersectionMemberFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeGeneric:
		id, ok := row.GenericBase()
		if !add(id, ok) {
			return nil, false
		}
		offset, count, ok := row.GenericArgumentSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeGenericArgumentFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeArray:
		id, ok := row.ArrayElement()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeMap:
		key, keyOK := row.MapKey()
		value, valueOK := row.MapValue()
		if !add(key, keyOK) || !add(value, valueOK) {
			return nil, false
		}
	case programschema.StaticNodeRecord:
		offset, count, ok := row.RecordFieldSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeRecordFieldFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeReference:
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
		if !sourceOK || !canonicalOK || !staticNodeMetadataSpanSealed(artifact, row.ID(), sourceOffset, sourceCount, programschema.StaticTypeNodeReferenceSourceKeyFamily()) || !staticNodeMetadataSpanSealed(artifact, row.ID(), canonicalOffset, canonicalCount, programschema.StaticTypeNodeReferenceCanonicalKeyFamily()) {
			return nil, false
		}
	case programschema.StaticNodeAlias:
		id, ok := row.AliasTarget()
		if !add(id, ok) {
			return nil, false
		}
		offset, count, ok := row.AliasParameterSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeAliasParameterFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeTypeParam:
		id, ok := row.TypeParamConstraint()
		if id.Available() && !optional(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeInterface:
		offset, count, ok := row.InterfaceExtendSpan()
		if !ok {
			return nil, false
		}
		extends, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeInterfaceExtendFamily())
		if !ok || !span(0, uint32(len(extends)), func(position uint32) (identity.ContentID, bool) { return extends[position].ChildID(), true }) {
			return nil, false
		}
		offset, count, ok = row.InterfaceMemberSpan()
		if !ok {
			return nil, false
		}
		members, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeInterfaceMemberFamily())
		if !ok || !span(0, uint32(len(members)), func(position uint32) (identity.ContentID, bool) { return members[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeTypeFunction:
		if id, ok := row.TypeFunctionVariadic(); ok && !add(id, true) {
			return nil, false
		}
		offset, count, ok := row.TypeFunctionTypeParameterSpan()
		if !ok {
			return nil, false
		}
		typeParams, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeTypeFunctionTypeParameterFamily())
		if !ok || !span(0, uint32(len(typeParams)), func(position uint32) (identity.ContentID, bool) { return typeParams[position].ChildID(), true }) {
			return nil, false
		}
		offset, count, ok = row.TypeFunctionParameterSpan()
		if !ok {
			return nil, false
		}
		params, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeTypeFunctionParameterFamily())
		if !ok || !span(0, uint32(len(params)), func(position uint32) (identity.ContentID, bool) { return params[position].ChildID(), true }) {
			return nil, false
		}
		offset, count, ok = row.TypeFunctionReturnSpan()
		if !ok {
			return nil, false
		}
		returns, ok := staticNodeFamilySpan(artifact, row.ID(), offset, count, programschema.StaticTypeNodeTypeFunctionReturnFamily())
		if !ok || !span(0, uint32(len(returns)), func(position uint32) (identity.ContentID, bool) { return returns[position].ChildID(), true }) {
			return nil, false
		}
	case programschema.StaticNodeKeyOf:
		id, ok := row.KeyOfChild()
		if !add(id, ok) {
			return nil, false
		}
	case programschema.StaticNodeIndex:
		object, objectOK := row.IndexObject()
		key, keyOK := row.IndexKey()
		if !add(object, objectOK) || !add(key, keyOK) {
			return nil, false
		}
	case programschema.StaticNodeConditional:
		a, b, c, d, ok := row.ConditionalChildren()
		if !ok || !add(a, true) || !add(b, true) || !add(c, true) || !add(d, true) {
			return nil, false
		}
	case programschema.StaticNodeAssertion:
		id, ok := row.AssertionNarrowID()
		if id.Available() && !optional(id, ok) {
			return nil, false
		}
	}
	return result, true
}
