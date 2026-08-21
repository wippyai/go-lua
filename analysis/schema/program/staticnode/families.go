package staticnode

import (
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
)

// The static catalog is bound here, beside the rows it owns. No root package
// accessor can create a second authority for one of these thirteen columns.
func StaticTypeNodeFamily() programfamily.Family[StaticTypeNode] {
	return programfamily.New[StaticTypeNode](programcatalog.StaticTypeNode())
}

func StaticTypeNodeUnionMemberFamily() programfamily.Family[StaticTypeNodeUnionMember] {
	return programfamily.New[StaticTypeNodeUnionMember](programcatalog.StaticTypeNodeUnionMember())
}

func StaticTypeNodeIntersectionMemberFamily() programfamily.Family[StaticTypeNodeIntersectionMember] {
	return programfamily.New[StaticTypeNodeIntersectionMember](programcatalog.StaticTypeNodeIntersectionMember())
}

func StaticTypeNodeGenericArgumentFamily() programfamily.Family[StaticTypeNodeGenericArgument] {
	return programfamily.New[StaticTypeNodeGenericArgument](programcatalog.StaticTypeNodeGenericArgument())
}

func StaticTypeNodeAliasParameterFamily() programfamily.Family[StaticTypeNodeAliasParameter] {
	return programfamily.New[StaticTypeNodeAliasParameter](programcatalog.StaticTypeNodeAliasParameter())
}

func StaticTypeNodeInterfaceExtendFamily() programfamily.Family[StaticTypeNodeInterfaceExtend] {
	return programfamily.New[StaticTypeNodeInterfaceExtend](programcatalog.StaticTypeNodeInterfaceExtend())
}

func StaticTypeNodeInterfaceMemberFamily() programfamily.Family[StaticTypeNodeInterfaceMember] {
	return programfamily.New[StaticTypeNodeInterfaceMember](programcatalog.StaticTypeNodeInterfaceMember())
}

func StaticTypeNodeTypeFunctionTypeParameterFamily() programfamily.Family[StaticTypeNodeTypeFunctionTypeParameter] {
	return programfamily.New[StaticTypeNodeTypeFunctionTypeParameter](programcatalog.StaticTypeNodeTypeFunctionTypeParameter())
}

func StaticTypeNodeTypeFunctionParameterFamily() programfamily.Family[StaticTypeNodeTypeFunctionParameter] {
	return programfamily.New[StaticTypeNodeTypeFunctionParameter](programcatalog.StaticTypeNodeTypeFunctionParameter())
}

func StaticTypeNodeTypeFunctionReturnFamily() programfamily.Family[StaticTypeNodeTypeFunctionReturn] {
	return programfamily.New[StaticTypeNodeTypeFunctionReturn](programcatalog.StaticTypeNodeTypeFunctionReturn())
}

func StaticTypeNodeRecordFieldFamily() programfamily.Family[StaticTypeNodeRecordField] {
	return programfamily.New[StaticTypeNodeRecordField](programcatalog.StaticTypeNodeRecordField())
}

func StaticTypeNodeReferenceSourceKeyFamily() programfamily.Family[StaticTypeNodeReferenceSourceKey] {
	return programfamily.New[StaticTypeNodeReferenceSourceKey](programcatalog.StaticTypeNodeReferenceSourceKey())
}

func StaticTypeNodeReferenceCanonicalKeyFamily() programfamily.Family[StaticTypeNodeReferenceCanonicalKey] {
	return programfamily.New[StaticTypeNodeReferenceCanonicalKey](programcatalog.StaticTypeNodeReferenceCanonicalKey())
}
