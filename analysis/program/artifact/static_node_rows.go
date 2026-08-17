package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// StaticNodeKind is the closed authored Static graph vocabulary.  Rows carry
// only owner-issued identities and scalar payload; local Terms never cross
// the ProgramArtifact boundary.
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

// StaticTypeNodeRow is one immutable authored Static node. Children are
// stable owner-fenced node IDs; scalar fields retain the exact authored
// disposition needed by typeauthority and Static evaluation.
type StaticTypeNodeRow struct {
	id       identity.ContentID
	owner    identity.ContentID
	kind     StaticNodeKind
	children []identity.ContentID
	// The typed slices below preserve declaration boundaries. children is kept
	// only for generic/union/intersection/operator edges; consumers must not
	// infer alias, interface, or function segments from one overloaded list.
	aliasParams            []identity.ContentID
	interfaceExtends       []identity.ContentID
	interfaceMemberTypes   []identity.ContentID
	typeFunctionVariadic   identity.ContentID
	typeFunctionParams     []identity.ContentID
	typeFunctionTypeParams []identity.ContentID
	typeFunctionReturns    []identity.ContentID
	declaration            identity.ContentID
	operand                identity.ContentID
	scope                  identity.ContentID
	assertionNarrow        identity.ContentID
	assertionCoordinate    [4]uint32
	keys                   []keyspace.Key
	texts                  []string
	optional               []bool
	fieldKeys              []keyspace.Key
	fieldTexts             []string
	fieldOptional          []bool
	fieldReadonly          []bool
	memberKinds            []uint8
	segments               []uint32
	returnsKnown           bool
	sourceKeys             []keyspace.Key
	canonicalKeys          []keyspace.Key
	assertParam            uint32
	name                   string
	key                    keyspace.Key
	exact                  keyspace.LiteralValue
	literal                uint8
	bits                   uint64
	flag                   bool
	resolution             uint8
}

func (row StaticTypeNodeRow) Available() bool {
	return row.id.Available() && row.owner.Available() && row.kind != StaticNodeInvalid && row.kind < StaticNodeUnknown
}
func (row StaticTypeNodeRow) ID() identity.ContentID    { return row.id }
func (row StaticTypeNodeRow) Owner() identity.ContentID { return row.owner }
func (row StaticTypeNodeRow) Kind() StaticNodeKind      { return row.kind }
func (row StaticTypeNodeRow) ChildCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.children)
}
func (row StaticTypeNodeRow) ChildAt(index int) (identity.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.children) {
		return identity.ContentID{}, false
	}
	return row.children[index], row.children[index].Available()
}
func (row StaticTypeNodeRow) AliasParamCount() int { return len(row.aliasParams) }
func (row StaticTypeNodeRow) AliasParamAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.aliasParams) {
		return identity.ContentID{}, false
	}
	return row.aliasParams[index], row.aliasParams[index].Available()
}
func (row StaticTypeNodeRow) InterfaceExtendCount() int { return len(row.interfaceExtends) }
func (row StaticTypeNodeRow) InterfaceExtendAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.interfaceExtends) {
		return identity.ContentID{}, false
	}
	return row.interfaceExtends[index], row.interfaceExtends[index].Available()
}
func (row StaticTypeNodeRow) InterfaceMemberTypeAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.interfaceMemberTypes) {
		return identity.ContentID{}, false
	}
	return row.interfaceMemberTypes[index], row.interfaceMemberTypes[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionVariadic() (identity.ContentID, bool) {
	return row.typeFunctionVariadic, row.typeFunctionVariadic.Available()
}
func (row StaticTypeNodeRow) TypeFunctionTypeParamCount() int { return len(row.typeFunctionTypeParams) }
func (row StaticTypeNodeRow) TypeFunctionTypeParamAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionTypeParams) {
		return identity.ContentID{}, false
	}
	return row.typeFunctionTypeParams[index], row.typeFunctionTypeParams[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionParamCount() int { return len(row.typeFunctionParams) }
func (row StaticTypeNodeRow) TypeFunctionParamAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionParams) {
		return identity.ContentID{}, false
	}
	return row.typeFunctionParams[index], row.typeFunctionParams[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionReturnCount() int { return len(row.typeFunctionReturns) }
func (row StaticTypeNodeRow) TypeFunctionReturnAt(index int) (identity.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionReturns) {
		return identity.ContentID{}, false
	}
	return row.typeFunctionReturns[index], row.typeFunctionReturns[index].Available()
}
func (row StaticTypeNodeRow) DeclarationOwner() (identity.ContentID, bool) {
	return row.declaration, row.declaration.Available()
}
func (row StaticTypeNodeRow) OperandID() (identity.ContentID, bool) {
	return row.operand, row.operand.Available()
}
func (row StaticTypeNodeRow) ScopeID() (identity.ContentID, bool) {
	return row.scope, row.scope.Available()
}
func (row StaticTypeNodeRow) AssertionNarrowID() (identity.ContentID, bool) {
	return row.assertionNarrow, row.assertionNarrow.Available()
}
func (row StaticTypeNodeRow) AssertionCoordinate() (uint32, uint32, uint32, uint32) {
	return row.assertionCoordinate[0], row.assertionCoordinate[1], row.assertionCoordinate[2], row.assertionCoordinate[3]
}
func (row StaticTypeNodeRow) KeyCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.keys)
}
func (row StaticTypeNodeRow) KeyAt(index int) (keyspace.Key, bool) {
	if !row.Available() || index < 0 || index >= len(row.keys) {
		return 0, false
	}
	return row.keys[index], row.keys[index] != 0
}
func (row StaticTypeNodeRow) TextAt(index int) (string, bool) {
	if !row.Available() || index < 0 || index >= len(row.texts) {
		return "", false
	}
	return row.texts[index], true
}
func (row StaticTypeNodeRow) OptionalAt(index int) (bool, bool) {
	if !row.Available() || index < 0 || index >= len(row.optional) {
		return false, false
	}
	return row.optional[index], true
}
func (row StaticTypeNodeRow) FieldCount() int { return len(row.fieldKeys) }
func (row StaticTypeNodeRow) FieldKeyAt(index int) (keyspace.Key, bool) {
	if index < 0 || index >= len(row.fieldKeys) {
		return 0, false
	}
	return row.fieldKeys[index], row.fieldKeys[index] != 0
}
func (row StaticTypeNodeRow) FieldTextAt(index int) (string, bool) {
	if index < 0 || index >= len(row.fieldTexts) {
		return "", false
	}
	return row.fieldTexts[index], true
}
func (row StaticTypeNodeRow) FieldOptionalAt(index int) (bool, bool) {
	if index < 0 || index >= len(row.fieldOptional) {
		return false, false
	}
	return row.fieldOptional[index], true
}
func (row StaticTypeNodeRow) FieldReadonlyAt(index int) (bool, bool) {
	if index < 0 || index >= len(row.fieldReadonly) {
		return false, false
	}
	return row.fieldReadonly[index], true
}
func (row StaticTypeNodeRow) MemberKindAt(index int) (uint8, bool) {
	if !row.Available() || index < 0 || index >= len(row.memberKinds) {
		return 0, false
	}
	return row.memberKinds[index], true
}
func (row StaticTypeNodeRow) SegmentCount() int { return len(row.segments) }
func (row StaticTypeNodeRow) SegmentAt(index int) (uint32, bool) {
	if index < 0 || index >= len(row.segments) {
		return 0, false
	}
	return row.segments[index], true
}
func (row StaticTypeNodeRow) ReturnsKnown() bool  { return row.returnsKnown }
func (row StaticTypeNodeRow) SourceKeyCount() int { return len(row.sourceKeys) }
func (row StaticTypeNodeRow) SourceKeyAt(index int) (keyspace.Key, bool) {
	if index < 0 || index >= len(row.sourceKeys) {
		return 0, false
	}
	return row.sourceKeys[index], row.sourceKeys[index] != 0
}
func (row StaticTypeNodeRow) CanonicalKeyCount() int { return len(row.canonicalKeys) }
func (row StaticTypeNodeRow) CanonicalKeyAt(index int) (keyspace.Key, bool) {
	if index < 0 || index >= len(row.canonicalKeys) {
		return 0, false
	}
	return row.canonicalKeys[index], row.canonicalKeys[index] != 0
}
func (row StaticTypeNodeRow) AssertionParam() uint32       { return row.assertParam }
func (row StaticTypeNodeRow) Name() string                 { return row.name }
func (row StaticTypeNodeRow) Key() keyspace.Key            { return row.key }
func (row StaticTypeNodeRow) Exact() keyspace.LiteralValue { return row.exact }
func (row StaticTypeNodeRow) LiteralKind() uint8           { return row.literal }
func (row StaticTypeNodeRow) Bits() uint64                 { return row.bits }
func (row StaticTypeNodeRow) Flag() bool                   { return row.flag }
func (row StaticTypeNodeRow) Resolution() uint8            { return row.resolution }
