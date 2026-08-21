package staticnode

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/snapshot"
)

// Publication is the construction-only payload for the static-node plane.
// It owns exactly the thirteen manifest columns at slots 42 through 54; the
// aggregate publication package is responsible for composing this plane with
// the other root-owned columns.
type Publication struct {
	StaticTypeNodes                          []StaticTypeNode
	StaticTypeNodeUnionMembers               []StaticTypeNodeUnionMember
	StaticTypeNodeIntersectionMembers        []StaticTypeNodeIntersectionMember
	StaticTypeNodeGenericArguments           []StaticTypeNodeGenericArgument
	StaticTypeNodeAliasParameters            []StaticTypeNodeAliasParameter
	StaticTypeNodeInterfaceExtends           []StaticTypeNodeInterfaceExtend
	StaticTypeNodeInterfaceMembers           []StaticTypeNodeInterfaceMember
	StaticTypeNodeTypeFunctionTypeParameters []StaticTypeNodeTypeFunctionTypeParameter
	StaticTypeNodeTypeFunctionParameters     []StaticTypeNodeTypeFunctionParameter
	StaticTypeNodeTypeFunctionReturns        []StaticTypeNodeTypeFunctionReturn
	StaticTypeNodeRecordFields               []StaticTypeNodeRecordField
	StaticTypeNodeReferenceSourceKeys        []StaticTypeNodeReferenceSourceKey
	StaticTypeNodeReferenceCanonicalKeys     []StaticTypeNodeReferenceCanonicalKey
}

// Append writes this plane in the canonical manifest order. It is deliberately
// construction-only: readers must enter through View over an authenticated
// programstate.State, never through a publication value.
func (publication Publication) Append(builder *snapshot.FrozenBuilder, catalogID identity.ContentID) bool {
	if builder == nil || !catalogID.Available() {
		return false
	}
	return StaticTypeNodeFamily().Put(builder, publication.StaticTypeNodes, catalogID) &&
		StaticTypeNodeUnionMemberFamily().Put(builder, publication.StaticTypeNodeUnionMembers, catalogID) &&
		StaticTypeNodeIntersectionMemberFamily().Put(builder, publication.StaticTypeNodeIntersectionMembers, catalogID) &&
		StaticTypeNodeGenericArgumentFamily().Put(builder, publication.StaticTypeNodeGenericArguments, catalogID) &&
		StaticTypeNodeAliasParameterFamily().Put(builder, publication.StaticTypeNodeAliasParameters, catalogID) &&
		StaticTypeNodeInterfaceExtendFamily().Put(builder, publication.StaticTypeNodeInterfaceExtends, catalogID) &&
		StaticTypeNodeInterfaceMemberFamily().Put(builder, publication.StaticTypeNodeInterfaceMembers, catalogID) &&
		StaticTypeNodeTypeFunctionTypeParameterFamily().Put(builder, publication.StaticTypeNodeTypeFunctionTypeParameters, catalogID) &&
		StaticTypeNodeTypeFunctionParameterFamily().Put(builder, publication.StaticTypeNodeTypeFunctionParameters, catalogID) &&
		StaticTypeNodeTypeFunctionReturnFamily().Put(builder, publication.StaticTypeNodeTypeFunctionReturns, catalogID) &&
		StaticTypeNodeRecordFieldFamily().Put(builder, publication.StaticTypeNodeRecordFields, catalogID) &&
		StaticTypeNodeReferenceSourceKeyFamily().Put(builder, publication.StaticTypeNodeReferenceSourceKeys, catalogID) &&
		StaticTypeNodeReferenceCanonicalKeyFamily().Put(builder, publication.StaticTypeNodeReferenceCanonicalKeys, catalogID)
}
