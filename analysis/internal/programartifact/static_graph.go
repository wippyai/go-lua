package programartifact

import (
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/keyspace"
	programstatic "github.com/wippyai/go-lua/program/static"
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
	id       keyspace.ContentID
	owner    keyspace.ContentID
	kind     StaticNodeKind
	children []keyspace.ContentID
	// The typed slices below preserve declaration boundaries. children is kept
	// only for generic/union/intersection/operator edges; consumers must not
	// infer alias, interface, or function segments from one overloaded list.
	aliasParams            []keyspace.ContentID
	interfaceExtends       []keyspace.ContentID
	interfaceMemberTypes   []keyspace.ContentID
	typeFunctionVariadic   keyspace.ContentID
	typeFunctionParams     []keyspace.ContentID
	typeFunctionTypeParams []keyspace.ContentID
	typeFunctionReturns    []keyspace.ContentID
	declaration            keyspace.ContentID
	operand                keyspace.ContentID
	scope                  keyspace.ContentID
	assertionNarrow        keyspace.ContentID
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

// StaticExpressionRow is the closed expression denominator for one mounted
// Program. Its identity and reference are both issued by Program.Static.
type StaticExpressionRow struct{ id, reference, owner keyspace.ContentID }

func (row StaticExpressionRow) Available() bool {
	return row.id.Available() && row.reference.Available() && row.owner.Available()
}
func (row StaticExpressionRow) ID() keyspace.ContentID          { return row.id }
func (row StaticExpressionRow) ReferenceID() keyspace.ContentID { return row.reference }
func (row StaticExpressionRow) Owner() keyspace.ContentID       { return row.owner }

type StaticInputKind uint8

const (
	StaticInputInvalid StaticInputKind = iota
	StaticInputTypeOf
	StaticInputAnnotation
)

// StaticInputOperandKind is the exact Program-issued operand disposition.
// Invalid is reserved for an unavailable row; the compiler rejects malformed
// authored operands instead of fabricating a fallback judgment.
type StaticInputOperandKind uint8

const (
	StaticInputOperandInvalid StaticInputOperandKind = iota
	StaticInputOperandKnown
	StaticInputOperandRuntimeSubject
	StaticInputOperandTypeValue
)

// StaticInputRow is the closed authored input denominator. The row uses the
// existing Program-issued semantic IDs for its expression and operand.
type StaticInputRow struct {
	id, owner, expression, source, target, operand, frontier keyspace.ContentID
	operandReference, operandSubject, operandBody            keyspace.ContentID
	literal                                                  keyspace.LiteralValue
	kind                                                     StaticInputKind
	operandKind                                              StaticInputOperandKind
	cursor                                                   uint32
}

func (row StaticInputRow) Available() bool {
	if !row.id.Available() || !row.owner.Available() || !row.expression.Available() || !row.source.Available() || !row.target.Available() || !row.operand.Available() || !row.frontier.Available() || row.kind == StaticInputInvalid || row.operandKind == StaticInputOperandInvalid {
		return false
	}
	switch row.operandKind {
	case StaticInputOperandKnown:
		return row.operandSubject == (keyspace.ContentID{}) && row.operandReference == (keyspace.ContentID{})
	case StaticInputOperandRuntimeSubject:
		return row.operandSubject.Available() && row.operandBody.Available() && row.operandReference == (keyspace.ContentID{})
	case StaticInputOperandTypeValue:
		return row.operandReference.Available() && row.operandBody.Available() && row.operandSubject == (keyspace.ContentID{})
	default:
		return false
	}
}
func (row StaticInputRow) ID() keyspace.ContentID                 { return row.id }
func (row StaticInputRow) Owner() keyspace.ContentID              { return row.owner }
func (row StaticInputRow) Kind() StaticInputKind                  { return row.kind }
func (row StaticInputRow) ExpressionID() keyspace.ContentID       { return row.expression }
func (row StaticInputRow) SourceID() keyspace.ContentID           { return row.source }
func (row StaticInputRow) TargetID() keyspace.ContentID           { return row.target }
func (row StaticInputRow) OperandID() keyspace.ContentID          { return row.operand }
func (row StaticInputRow) FrontierID() keyspace.ContentID         { return row.frontier }
func (row StaticInputRow) Cursor() uint32                         { return row.cursor }
func (row StaticInputRow) OperandKind() StaticInputOperandKind    { return row.operandKind }
func (row StaticInputRow) OperandLiteral() keyspace.LiteralValue  { return row.literal }
func (row StaticInputRow) OperandReferenceID() keyspace.ContentID { return row.operandReference }
func (row StaticInputRow) OperandSubjectID() keyspace.ContentID   { return row.operandSubject }
func (row StaticInputRow) OperandBodyPathID() keyspace.ContentID  { return row.operandBody }

func (row StaticTypeNodeRow) Available() bool {
	return row.id.Available() && row.owner.Available() && row.kind != StaticNodeInvalid && row.kind < StaticNodeUnknown
}
func (row StaticTypeNodeRow) ID() keyspace.ContentID    { return row.id }
func (row StaticTypeNodeRow) Owner() keyspace.ContentID { return row.owner }
func (row StaticTypeNodeRow) Kind() StaticNodeKind      { return row.kind }
func (row StaticTypeNodeRow) ChildCount() int {
	if !row.Available() {
		return 0
	}
	return len(row.children)
}
func (row StaticTypeNodeRow) ChildAt(index int) (keyspace.ContentID, bool) {
	if !row.Available() || index < 0 || index >= len(row.children) {
		return keyspace.ContentID{}, false
	}
	return row.children[index], row.children[index].Available()
}
func (row StaticTypeNodeRow) AliasParamCount() int { return len(row.aliasParams) }
func (row StaticTypeNodeRow) AliasParamAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.aliasParams) {
		return keyspace.ContentID{}, false
	}
	return row.aliasParams[index], row.aliasParams[index].Available()
}
func (row StaticTypeNodeRow) InterfaceExtendCount() int { return len(row.interfaceExtends) }
func (row StaticTypeNodeRow) InterfaceExtendAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.interfaceExtends) {
		return keyspace.ContentID{}, false
	}
	return row.interfaceExtends[index], row.interfaceExtends[index].Available()
}
func (row StaticTypeNodeRow) InterfaceMemberTypeAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.interfaceMemberTypes) {
		return keyspace.ContentID{}, false
	}
	return row.interfaceMemberTypes[index], row.interfaceMemberTypes[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionVariadic() (keyspace.ContentID, bool) {
	return row.typeFunctionVariadic, row.typeFunctionVariadic.Available()
}
func (row StaticTypeNodeRow) TypeFunctionTypeParamCount() int { return len(row.typeFunctionTypeParams) }
func (row StaticTypeNodeRow) TypeFunctionTypeParamAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionTypeParams) {
		return keyspace.ContentID{}, false
	}
	return row.typeFunctionTypeParams[index], row.typeFunctionTypeParams[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionParamCount() int { return len(row.typeFunctionParams) }
func (row StaticTypeNodeRow) TypeFunctionParamAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionParams) {
		return keyspace.ContentID{}, false
	}
	return row.typeFunctionParams[index], row.typeFunctionParams[index].Available()
}
func (row StaticTypeNodeRow) TypeFunctionReturnCount() int { return len(row.typeFunctionReturns) }
func (row StaticTypeNodeRow) TypeFunctionReturnAt(index int) (keyspace.ContentID, bool) {
	if index < 0 || index >= len(row.typeFunctionReturns) {
		return keyspace.ContentID{}, false
	}
	return row.typeFunctionReturns[index], row.typeFunctionReturns[index].Available()
}
func (row StaticTypeNodeRow) DeclarationOwner() (keyspace.ContentID, bool) {
	return row.declaration, row.declaration.Available()
}
func (row StaticTypeNodeRow) OperandID() (keyspace.ContentID, bool) {
	return row.operand, row.operand.Available()
}
func (row StaticTypeNodeRow) ScopeID() (keyspace.ContentID, bool) {
	return row.scope, row.scope.Available()
}
func (row StaticTypeNodeRow) AssertionNarrowID() (keyspace.ContentID, bool) {
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

func staticNodeID(owner keyspace.ContentID, ref programstatic.StaticTypeRef) (id keyspace.ContentID, ok bool) {
	if !owner.Available() || ref.Term() == 0 {
		return keyspace.ContentID{}, false
	}
	return program.StaticTypeReferenceID(owner, ref)
}

func (compiler *compiler) copyStaticGraphFailure() CompileFailure {
	if !compiler.input.Available() {
		return compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	owner := compiler.input.ContentID()
	ownerIDForRow := owner
	view := compiler.input.Static()
	rows := make([]StaticTypeNodeRow, 0, view.StaticTypes().Count())
	compiler.staticExpressions = make([]StaticExpressionRow, 0, view.StaticTypes().Count())
	compiler.staticInputs = make([]StaticInputRow, 0, compiler.input.StaticTypeOfCount())
	operandRow := func(term keyspace.Term) (StaticInputOperandKind, keyspace.LiteralValue, keyspace.ContentID, keyspace.ContentID, keyspace.ContentID, keyspace.ContentID, bool) {
		operand, ok := compiler.input.StaticOperandAt(term)
		if !ok {
			return StaticInputOperandInvalid, keyspace.LiteralValue{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
		}
		kind := StaticInputOperandInvalid
		switch operand.Kind() {
		case program.StaticOperandKnown:
			kind = StaticInputOperandKnown
		case program.StaticOperandRuntimeSubject:
			kind = StaticInputOperandRuntimeSubject
		case program.StaticOperandTypeValue:
			kind = StaticInputOperandTypeValue
		default:
			return StaticInputOperandInvalid, keyspace.LiteralValue{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, keyspace.ContentID{}, false
		}
		return kind, operand.Literal(), operand.ID(), operand.ReferenceID(), operand.SubjectID(), operand.BodyPathID(), true
	}
	expressionIDs := make(map[keyspace.Term]keyspace.ContentID, view.StaticTypes().Count())
	for expressionIndex := 0; expressionIndex < view.StaticTypes().Count(); expressionIndex++ {
		ref, refOK := view.StaticTypes().At(expressionIndex)
		if !refOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		nodeID, nodeOK := staticNodeID(owner, ref)
		expressionID, expressionOK := program.StaticExpressionID(owner, ref)
		if !nodeOK || !expressionOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		expressionIDs[ref.Term()] = expressionID
		compiler.staticExpressions = append(compiler.staticExpressions, StaticExpressionRow{id: expressionID, reference: nodeID, owner: owner})
	}
	for inputIndex := 0; inputIndex < compiler.input.StaticTypeOfCount(); inputIndex++ {
		sourceTerm, operandTerm, inputOK := compiler.input.StaticTypeOfAt(inputIndex)
		if !inputOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
		}
		expressionRef, expressionOK := view.StaticTypes().Ref(sourceTerm)
		sourceID, sourceOK := staticNodeID(owner, expressionRef)
		operandID, operandOK := program.StaticOccurrenceID(owner, 1, operandTerm)
		operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
		if semanticOperandID.Available() {
			operandID = semanticOperandID
		}
		frontierID, cursor, frontierOK := compiler.input.StaticFrontier(sourceTerm)
		if !expressionOK || !sourceOK || !operandOK || !frontierOK || !dispositionOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
		}
		rowID, rowIDOK := program.StaticInputID(owner, 2, sourceTerm, uint32(inputIndex))
		if !rowIDOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
		}
		expressionID, expressionIDOK := expressionIDs[sourceTerm]
		if !expressionIDOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
		}
		compiler.staticInputs = append(compiler.staticInputs, StaticInputRow{id: rowID, owner: owner, expression: expressionID, source: sourceID, target: sourceID, operand: operandID, frontier: frontierID, kind: StaticInputTypeOf, operandKind: operandKind, literal: literal, operandReference: operandReference, operandSubject: operandSubject, operandBody: operandBody, cursor: cursor})
	}
	for annotationIndex := 0; annotationIndex < compiler.input.StaticAnnotationCount(); annotationIndex++ {
		sourceTerm, targetTerm, valuesTerm, annotationOK := compiler.input.StaticAnnotationAt(annotationIndex)
		if !annotationOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		count, countOK := compiler.input.StaticAnnotationValueCount(valuesTerm)
		if !countOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		targetRef, targetRefOK := view.StaticTypes().Ref(targetTerm)
		targetID, targetIDOK := staticNodeID(owner, targetRef)
		frontierID, cursor, frontierOK := compiler.input.StaticFrontier(sourceTerm)
		if !targetRefOK || !targetIDOK || !frontierOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		annotationSourceID, annotationSourceOK := program.StaticOccurrenceID(owner, 5, sourceTerm)
		if !annotationSourceOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		for valueIndex := 0; valueIndex < count; valueIndex++ {
			operandTerm, operandOK := compiler.input.StaticAnnotationValue(valuesTerm, valueIndex)
			operandID, operandIDOK := program.StaticOccurrenceID(owner, 3, operandTerm)
			operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
			if semanticOperandID.Available() {
				operandID = semanticOperandID
			}
			rowID, rowIDOK := program.StaticInputID(owner, 5, sourceTerm, uint32(valueIndex))
			if !operandOK || !operandIDOK || !rowIDOK || !dispositionOK {
				return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, valueIndex, CompileReasonProgramUnavailable)
			}
			expressionID, expressionIDOK := expressionIDs[targetTerm]
			if !expressionIDOK {
				return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, valueIndex, CompileReasonProgramUnavailable)
			}
			compiler.staticInputs = append(compiler.staticInputs, StaticInputRow{id: rowID, owner: owner, expression: expressionID, source: annotationSourceID, target: targetID, operand: operandID, frontier: frontierID, kind: StaticInputAnnotation, operandKind: operandKind, literal: literal, operandReference: operandReference, operandSubject: operandSubject, operandBody: operandBody, cursor: cursor})
		}
	}
	for index := 0; index < view.StaticTypes().Count(); index++ {
		ref, ok := view.StaticTypes().At(index)
		if !ok {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, index, -1, CompileReasonProgramUnavailable)
		}
		term := ref.Term()
		id, ok := staticNodeID(owner, ref)
		if !ok {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, index, -1, CompileReasonProgramUnavailable)
		}
		row := StaticTypeNodeRow{id: id, owner: owner, kind: StaticNodeUnknown}
		childID := func(child keyspace.Term) (keyspace.ContentID, bool) {
			childRef, childRefOK := view.StaticTypes().Ref(child)
			if !childRefOK {
				return keyspace.ContentID{}, false
			}
			return staticNodeID(owner, childRef)
		}
		appendChild := func(child keyspace.Term) bool {
			id, childOK := childID(child)
			if !childOK {
				return false
			}
			row.children = append(row.children, id)
			return true
		}
		if primitive, found := view.Types().Primitives().Get(term); found {
			row.kind, row.literal = StaticNodePrimitive, uint8(primitive)
		} else if literal, key, bits, found := view.Types().Literals().Get(term); found {
			row.kind, row.literal, row.key, row.bits = StaticNodeLiteral, uint8(literal), key, bits
			row.exact, _ = compiler.input.StaticKeyLiteral(key)
			row.name, _ = compiler.input.StaticKeyText(key)
		} else if child, found := view.Types().Optionals().Get(term); found {
			row.kind = StaticNodeOptional
			ok = appendChild(child)
		} else if count, found := view.Types().Unions().MemberCount(term); found {
			row.kind = StaticNodeUnion
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Unions().MemberAt(term, n)
				ok = childOK && appendChild(child)
			}
		} else if count, found := view.Types().Intersections().MemberCount(term); found {
			row.kind = StaticNodeIntersection
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Intersections().MemberAt(term, n)
				ok = childOK && appendChild(child)
			}
		} else if base, count, found := view.Types().Generics().Get(term); found {
			row.kind = StaticNodeGeneric
			ok = appendChild(base)
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Generics().ArgAt(term, n)
				ok = childOK && appendChild(child)
			}
		} else if child, readonly, found := view.Types().Arrays().Get(term); found {
			row.kind, row.flag = StaticNodeArray, readonly
			ok = appendChild(child)
		} else if key, value, readonly, found := view.Types().Maps().Get(term); found {
			row.kind, row.flag = StaticNodeMap, readonly
			ok = appendChild(key) && appendChild(value)
		} else if readonly, count, found := view.Types().Records().Get(term); found {
			row.kind, row.flag = StaticNodeRecord, readonly
			for n := 0; n < count && ok; n++ {
				field, fieldOK := view.Types().Records().FieldAt(term, n)
				fieldKey, fieldType, optional, fieldShapeOK := view.Types().Fields().Get(field)
				if fieldShapeOK {
					row.keys = append(row.keys, fieldKey)
					text, _ := compiler.input.StaticKeyText(fieldKey)
					row.texts = append(row.texts, text)
					row.optional = append(row.optional, optional)
					row.fieldKeys = append(row.fieldKeys, fieldKey)
					row.fieldTexts = append(row.fieldTexts, text)
					row.fieldOptional = append(row.fieldOptional, optional)
					row.fieldReadonly = append(row.fieldReadonly, readonly)
				}
				ok = fieldOK && fieldShapeOK && appendChild(fieldType)
			}
		} else if resolution, target, _, found := view.References().Get(term); found {
			row.kind, row.resolution = StaticNodeReference, uint8(resolution)
			if count, countOK := view.References().SourceCount(term); countOK {
				for n := 0; n < count; n++ {
					key, keyOK := view.References().SourceAt(term, n)
					if !keyOK {
						ok = false
						break
					}
					row.sourceKeys = append(row.sourceKeys, key)
				}
			}
			if count, countOK := view.References().CanonicalCount(term); countOK {
				for n := 0; n < count && ok; n++ {
					key, keyOK := view.References().CanonicalAt(term, n)
					if !keyOK {
						ok = false
						break
					}
					row.canonicalKeys = append(row.canonicalKeys, key)
				}
			}
			if target != 0 {
				ok = appendChild(target)
			}
			if ok && resolution == programstatic.TypeRefUnresolved {
				observation, observationOK := compiler.input.TypeReferenceUnresolvedObservation(term)
				ok = observationOK && compiler.admitDiagnosticObservation(observation)
			}
		} else if _, target, nameKey, _, found := view.Declarations().Aliases().Get(term); found {
			row.kind = StaticNodeAlias
			row.key = nameKey
			row.name, _ = compiler.input.StaticKeyText(nameKey)
			ok = appendChild(target)
			if count, countOK := view.Declarations().Aliases().ParamCount(term); countOK {
				row.segments = append(row.segments, uint32(count))
				for n := 0; n < count && ok; n++ {
					param, paramOK := view.Declarations().Aliases().ParamAt(term, n)
					paramID, idOK := childID(param)
					ok = paramOK && idOK
					if ok {
						row.aliasParams = append(row.aliasParams, paramID)
					}
					if ok {
						row.children = append(row.children, paramID)
					}
				}
			}
		} else if declOwner, nameKey, constraint, found := view.Declarations().TypeParams().Get(term); found {
			row.kind = StaticNodeTypeParam
			row.key = nameKey
			row.name, _ = compiler.input.StaticKeyText(nameKey)
			if declOwner != 0 {
				row.declaration, ok = program.StaticScopeID(ownerIDForRow, declOwner)
			}
			if constraint != 0 && ok {
				ok = appendChild(constraint)
			}
		} else if interfaceOwner, nameKey, _, found := view.Declarations().Interfaces().Get(term); found {
			row.kind = StaticNodeInterface
			row.key = nameKey
			row.name, _ = compiler.input.StaticKeyText(nameKey)
			if interfaceOwner != 0 {
				row.declaration, ok = program.StaticScopeID(ownerIDForRow, interfaceOwner)
			}
			if count, shapeOK := view.Declarations().Interfaces().ExtendCount(term); shapeOK {
				row.segments = append(row.segments, uint32(count))
				for n := 0; n < count && ok; n++ {
					child, childOK := view.Declarations().Interfaces().ExtendAt(term, n)
					childID, idOK := childID(child)
					ok = childOK && idOK
					if ok {
						row.interfaceExtends = append(row.interfaceExtends, childID)
						row.children = append(row.children, childID)
					}
				}
			}
			if count, shapeOK := view.Declarations().Interfaces().MemberCount(term); shapeOK {
				row.segments = append(row.segments, uint32(count))
				for n := 0; n < count && ok; n++ {
					member, memberOK := view.Declarations().Interfaces().MemberAt(term, n)
					if !memberOK {
						ok = false
						break
					}
					row.memberKinds = append(row.memberKinds, uint8(member.Kind))
					memberKey := member.Name
					memberType := member.Signature
					memberOptional := false
					if member.Kind == programstatic.InterfaceField && member.Field != 0 {
						fieldKey, fieldType, optional, fieldOK := view.Types().Fields().Get(member.Field)
						if !fieldOK {
							ok = false
							break
						}
						memberKey, memberType, memberOptional = fieldKey, fieldType, optional
					}
					row.keys = append(row.keys, memberKey)
					memberText, _ := compiler.input.StaticKeyText(memberKey)
					row.texts = append(row.texts, memberText)
					row.optional = append(row.optional, memberOptional)
					row.fieldKeys = append(row.fieldKeys, memberKey)
					row.fieldTexts = append(row.fieldTexts, memberText)
					row.fieldOptional = append(row.fieldOptional, memberOptional)
					row.fieldReadonly = append(row.fieldReadonly, false)
					memberID, idOK := childID(memberType)
					ok = idOK
					if ok {
						row.interfaceMemberTypes = append(row.interfaceMemberTypes, memberID)
						row.children = append(row.children, memberID)
					}
				}
			}
		} else if scope, variadic, _, returnsKnown, found := view.Signatures().TypeFunctions().Get(term); found {
			row.kind = StaticNodeTypeFunction
			row.flag = variadic != 0
			row.returnsKnown = returnsKnown
			row.scope, ok = program.StaticScopeID(owner, scope)
			if variadic != 0 {
				variadicID, idOK := childID(variadic)
				ok = idOK
				if ok {
					row.typeFunctionVariadic = variadicID
					row.children = append(row.children, variadicID)
				}
				row.segments = append(row.segments, 1)
			} else {
				row.segments = append(row.segments, 0)
			}
			if count, shapeOK := view.Signatures().TypeFunctions().TypeParamCount(term); shapeOK {
				row.segments = append(row.segments, uint32(count))
				for n := 0; n < count && ok; n++ {
					child, childOK := view.Signatures().TypeFunctions().TypeParamAt(term, n)
					childID, idOK := childID(child)
					ok = childOK && idOK
					if ok {
						row.typeFunctionTypeParams = append(row.typeFunctionTypeParams, childID)
						row.children = append(row.children, childID)
					}
				}
			}
			if count, shapeOK := view.Signatures().TypeFunctions().ParameterCount(term); shapeOK {
				row.segments = append(row.segments, uint32(count))
				for n := 0; n < count && ok; n++ {
					parameter, parameterOK := view.Signatures().TypeFunctions().ParameterAt(term, n)
					if !parameterOK {
						ok = false
						break
					}
					row.keys = append(row.keys, parameter.Name)
					parameterText, _ := compiler.input.StaticKeyText(parameter.Name)
					row.texts = append(row.texts, parameterText)
					parameterID, idOK := childID(parameter.Type)
					ok = idOK
					if ok {
						row.typeFunctionParams = append(row.typeFunctionParams, parameterID)
						row.children = append(row.children, parameterID)
					}
				}
			}
			if returnsKnown {
				if count, shapeOK := view.Signatures().TypeFunctions().ReturnCount(term); shapeOK {
					row.segments = append(row.segments, uint32(count))
					for n := 0; n < count && ok; n++ {
						child, childOK := view.Signatures().TypeFunctions().ReturnAt(term, n)
						childID, idOK := childID(child)
						ok = childOK && idOK
						if ok {
							row.typeFunctionReturns = append(row.typeFunctionReturns, childID)
							row.children = append(row.children, childID)
						}
					}
				}
			}
		} else if _, operand, found := view.Operators().TypeOfs().Get(term); found {
			row.kind = StaticNodeTypeOf
			row.operand, ok = program.StaticOccurrenceID(owner, 1, operand)
		} else if child, found := view.Operators().KeyOfs().Get(term); found {
			row.kind = StaticNodeKeyOf
			ok = appendChild(child)
		} else if object, indexTerm, found := view.Operators().IndexAccesses().Get(term); found {
			row.kind = StaticNodeIndex
			ok = appendChild(object) && appendChild(indexTerm)
		} else if check, extends, thenTerm, otherwise, found := view.Operators().Conditionals().Get(term); found {
			row.kind = StaticNodeConditional
			ok = appendChild(check) && appendChild(extends) && appendChild(thenTerm) && appendChild(otherwise)
		} else if nameKey, coordinate, bound, param, narrow, found := view.Signatures().Assertions().Get(term); found {
			row.kind = StaticNodeAssertion
			row.key = nameKey
			row.name, _ = compiler.input.StaticKeyText(nameKey)
			row.flag = bound
			row.assertParam = param
			if narrow != 0 {
				row.assertionNarrow, ok = childID(narrow)
				if ok {
					row.children = append(row.children, row.assertionNarrow)
				}
			}
			row.assertionCoordinate[0], row.assertionCoordinate[1], row.assertionCoordinate[2], row.assertionCoordinate[3] = coordinate.Parts()
		}
		if row.kind == StaticNodeUnknown {
			ok = false
		}
		if !ok {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, index, -1, CompileReasonProgramUnavailable)
		}
		rows = append(rows, row)
	}
	compiler.staticTypeNodes = rows
	return CompileFailure{}
}
