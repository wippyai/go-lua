package staticnode

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

type artifactIdentityChildRow interface {
	programfamily.Row
	ParentID() identity.ContentID
	ChildID() identity.ContentID
	Position() uint32
}

type artifactIdentityMetadata struct {
	key      uint64
	text     string
	optional bool
	kind     uint8
}

// writeArtifactIdentityChildFamily replays one typed dense child span in the
// historical static-node identity order. Each caller names its concrete
// canonical family; no generic child-plane projection is retained.
func writeArtifactIdentityChildFamily[V artifactIdentityChildRow](writer identity.StringIdentityWriter, view View, parent identity.ContentID, offset, count uint32, family programfamily.Family[V]) bool {
	if !writer.WriteUint(uint64(count)) {
		return false
	}
	for position := uint32(0); position < count; position++ {
		child, ok := staticTypeNodeFamilyAt(view, family, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position || !writer.WriteContentID(child.ChildID()) {
			return false
		}
	}
	return true
}

func writeArtifactIdentityReadonlyFamily[V artifactIdentityChildRow](writer identity.StringIdentityWriter, view View, parent identity.ContentID, offset, count uint32, family programfamily.Family[V], readonly func(V) bool) bool {
	if !writer.WriteUint(uint64(count)) {
		return false
	}
	for position := uint32(0); position < count; position++ {
		child, ok := staticTypeNodeFamilyAt(view, family, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position || !writer.WriteBool(readonly(child)) {
			return false
		}
	}
	return true
}

// writeArtifactIdentityMetadataSpan preserves the historical two-pass
// preimage: all metadata keys precede every text/optionality/kind payload.
func writeArtifactIdentityMetadataSpan[V artifactIdentityChildRow](writer identity.StringIdentityWriter, view View, parent identity.ContentID, offset, count uint32, family programfamily.Family[V], read func(V) artifactIdentityMetadata) bool {
	rows := make([]artifactIdentityMetadata, 0, count)
	for position := uint32(0); position < count; position++ {
		child, ok := staticTypeNodeFamilyAt(view, family, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position {
			return false
		}
		rows = append(rows, read(child))
	}
	if !writer.WriteUint(uint64(len(rows))) {
		return false
	}
	for _, row := range rows {
		if !writer.WriteUint(row.key) {
			return false
		}
	}
	for _, row := range rows {
		if !writer.WriteString(row.text) || !writer.WriteBool(row.optional) || !writer.WriteUint(uint64(row.kind)) {
			return false
		}
	}
	return true
}

// WriteArtifactIdentityFields replays the canonical static-type node graph
// into an Artifact identity stream. It retains the historical field order:
// dense offsets are validated but never written, while semantic child order
// is reconstructed by the owner View.
func (view View) WriteArtifactIdentityFields(writer identity.StringIdentityWriter) bool {
	if writer == nil || !view.Available() {
		return false
	}
	typeNodeCount, typeNodesPublished := view.StaticTypeNodeCount()
	if !typeNodesPublished || !writer.WriteUint(uint64(typeNodeCount)) {
		return false
	}
	for index := 0; index < typeNodeCount; index++ {
		row, held := view.StaticTypeNodeAt(index)
		if !held || !row.Available() {
			return false
		}
		exact := row.Exact()
		if !writer.WriteContentID(row.ID()) || !writer.WriteContentID(row.Owner()) || !writer.WriteUint(uint64(row.Kind())) ||
			!writer.WriteString(row.Name()) || !writer.WriteUint(uint64(row.Key())) || !writer.WriteUint(uint64(row.LiteralKind())) ||
			!writer.WriteUint(row.Bits()) || !writer.WriteUint(uint64(exact.Kind)) || !writer.WriteBool(exact.Bool) ||
			!writer.WriteUint(uint64(exact.Integer)) || !writer.WriteUint(exact.FloatBits) || !writer.WriteString(exact.String) ||
			!writer.WriteBool(row.Flag()) || !writer.WriteUint(uint64(row.Resolution())) || !writer.WriteUint(uint64(row.AssertionParam())) {
			return false
		}
		declaration, _ := row.DeclarationOwner()
		operand, _ := row.OperandID()
		scope, _ := row.ScopeID()
		narrow, _ := row.AssertionNarrowID()
		variadic, _ := row.TypeFunctionVariadic()
		c0, c1, c2, c3 := row.AssertionCoordinate()
		if !writer.WriteContentID(declaration) || !writer.WriteContentID(operand) || !writer.WriteContentID(scope) ||
			!writer.WriteContentID(narrow) || !writer.WriteUint(uint64(c0)) || !writer.WriteUint(uint64(c1)) ||
			!writer.WriteUint(uint64(c2)) || !writer.WriteUint(uint64(c3)) || !writer.WriteContentID(variadic) {
			return false
		}
		aliasOffset, aliasCount, aliasOK := row.AliasParameterSpan()
		extendOffset, extendCount, extendOK := row.InterfaceExtendSpan()
		memberOffset, memberCount, memberOK := row.InterfaceMemberSpan()
		typeParamOffset, typeParamCount, typeParamOK := row.TypeFunctionTypeParameterSpan()
		parameterOffset, parameterCount, parameterOK := row.TypeFunctionParameterSpan()
		returnOffset, returnCount, returnOK := row.TypeFunctionReturnSpan()
		recordOffset, recordCount, recordOK := row.RecordFieldSpan()
		if !aliasOK || !extendOK || !memberOK || !typeParamOK || !parameterOK || !returnOK || !recordOK ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), aliasOffset, aliasCount, StaticTypeNodeAliasParameterFamily()) ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), extendOffset, extendCount, StaticTypeNodeInterfaceExtendFamily()) ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), memberOffset, memberCount, StaticTypeNodeInterfaceMemberFamily()) ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), typeParamOffset, typeParamCount, StaticTypeNodeTypeFunctionTypeParameterFamily()) ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), parameterOffset, parameterCount, StaticTypeNodeTypeFunctionParameterFamily()) ||
			!writeArtifactIdentityChildFamily(writer, view, row.ID(), returnOffset, returnCount, StaticTypeNodeTypeFunctionReturnFamily()) {
			return false
		}

		fieldReadonlyCount := uint32(0)
		if row.Kind() == StaticNodeRecord {
			fieldReadonlyCount = recordCount
		} else if row.Kind() == StaticNodeInterface {
			fieldReadonlyCount = memberCount
		}
		if row.Kind() == StaticNodeRecord {
			if !writeArtifactIdentityReadonlyFamily(writer, view, row.ID(), recordOffset, fieldReadonlyCount, StaticTypeNodeRecordFieldFamily(), func(field StaticTypeNodeRecordField) bool { return field.Readonly() }) {
				return false
			}
		} else if row.Kind() == StaticNodeInterface {
			if !writeArtifactIdentityReadonlyFamily(writer, view, row.ID(), memberOffset, fieldReadonlyCount, StaticTypeNodeInterfaceMemberFamily(), func(member StaticTypeNodeInterfaceMember) bool { return member.Readonly() }) {
				return false
			}
		} else if !writer.WriteUint(uint64(fieldReadonlyCount)) {
			return false
		}

		switch row.Kind() {
		case StaticNodeRecord:
			if !writeArtifactIdentityMetadataSpan(writer, view, row.ID(), recordOffset, recordCount, StaticTypeNodeRecordFieldFamily(), func(field StaticTypeNodeRecordField) artifactIdentityMetadata {
				return artifactIdentityMetadata{key: uint64(field.Key()), text: field.Text(), optional: field.Optional()}
			}) {
				return false
			}
		case StaticNodeInterface:
			if !writeArtifactIdentityMetadataSpan(writer, view, row.ID(), memberOffset, memberCount, StaticTypeNodeInterfaceMemberFamily(), func(member StaticTypeNodeInterfaceMember) artifactIdentityMetadata {
				return artifactIdentityMetadata{key: uint64(member.Key()), text: member.Text(), optional: member.Optional(), kind: member.KindCode()}
			}) {
				return false
			}
		case StaticNodeTypeFunction:
			if !writeArtifactIdentityMetadataSpan(writer, view, row.ID(), parameterOffset, parameterCount, StaticTypeNodeTypeFunctionParameterFamily(), func(parameter StaticTypeNodeTypeFunctionParameter) artifactIdentityMetadata {
				return artifactIdentityMetadata{key: uint64(parameter.Key()), text: parameter.Text()}
			}) {
				return false
			}
		default:
			if !writer.WriteUint(0) {
				return false
			}
		}

		segmentCount := row.SegmentCount()
		if !writer.WriteUint(uint64(segmentCount)) {
			return false
		}
		for segmentIndex := 0; segmentIndex < int(segmentCount); segmentIndex++ {
			segment, segmentOK := row.SegmentAt(segmentIndex)
			if !segmentOK || !writer.WriteUint(uint64(segment)) {
				return false
			}
		}
		if !writer.WriteBool(row.ReturnsKnown()) {
			return false
		}
		sourceOffset, sourceCount, sourceOK := row.ReferenceSourceKeySpan()
		canonicalOffset, canonicalCount, canonicalOK := row.ReferenceCanonicalKeySpan()
		if !sourceOK || !canonicalOK || !writer.WriteUint(uint64(sourceCount)) {
			return false
		}
		for keyIndex := uint32(0); keyIndex < sourceCount; keyIndex++ {
			key, keyOK := staticTypeNodeFamilyAt(view, StaticTypeNodeReferenceSourceKeyFamily(), int(sourceOffset+keyIndex))
			if !keyOK || key.ParentID() != row.ID() || !writer.WriteUint(uint64(key.Key())) {
				return false
			}
		}
		if !writer.WriteUint(uint64(canonicalCount)) {
			return false
		}
		for keyIndex := uint32(0); keyIndex < canonicalCount; keyIndex++ {
			key, keyOK := staticTypeNodeFamilyAt(view, StaticTypeNodeReferenceCanonicalKeyFamily(), int(canonicalOffset+keyIndex))
			if !keyOK || key.ParentID() != row.ID() || !writer.WriteUint(uint64(key.Key())) {
				return false
			}
		}
		children, childrenOK := view.StaticTypeNodeChildren(index, row, false)
		if !childrenOK || !writer.WriteUint(uint64(len(children))) {
			return false
		}
		for _, child := range children {
			if !writer.WriteContentID(child) {
				return false
			}
		}
	}
	return true
}
