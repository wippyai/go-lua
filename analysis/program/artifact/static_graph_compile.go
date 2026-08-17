package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programstatic "github.com/wippyai/go-lua/analysis/program/static"
)

func staticNodeID(owner identity.ContentID, ref programstatic.StaticTypeRef) (id identity.ContentID, ok bool) {
	if !owner.Available() || ref.Term() == 0 {
		return identity.ContentID{}, false
	}
	return programstatic.TypeReferenceID(owner, ref)
}

func (compiler *compiler) copyStaticGraphFailure() CompileFailure {
	if !compiler.input.Available() {
		return compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	owner := compiler.input.ContentID()
	ownerIDForRow := owner
	ownerProgram := compiler.input
	if ownerProgram == nil {
		return compileFailure(CompileStageAuthority, CompileRowAuthority, -1, -1, CompileReasonProgramUnavailable)
	}
	view := ownerProgram.Static()
	rows := make([]StaticTypeNodeRow, 0, view.StaticTypes().Count())
	compiler.staticExpressions = make([]StaticExpressionRow, 0, view.StaticTypes().Count())
	typeOfs := view.Operators().TypeOfs()
	annotations := view.Operands().Annotations()
	compiler.staticInputs = make([]StaticInputRow, 0, typeOfs.Count())
	operandRow := func(term keyspace.Term) (StaticInputOperandKind, keyspace.LiteralValue, identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID, bool) {
		operand, ok := ownerProgram.StaticOperandAt(term)
		if !ok {
			return StaticInputOperandInvalid, keyspace.LiteralValue{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
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
			return StaticInputOperandInvalid, keyspace.LiteralValue{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
		}
		return kind, operand.Literal(), operand.ID(), operand.ReferenceID(), operand.SubjectID(), operand.BodyPathID(), true
	}
	expressionIDs := make(map[keyspace.Term]identity.ContentID, view.StaticTypes().Count())
	for expressionIndex := 0; expressionIndex < view.StaticTypes().Count(); expressionIndex++ {
		ref, refOK := view.StaticTypes().At(expressionIndex)
		if !refOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		nodeID, nodeOK := staticNodeID(owner, ref)
		expressionID, expressionOK := programstatic.ExpressionID(owner, ref)
		if !nodeOK || !expressionOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		expressionIDs[ref.Term()] = expressionID
		compiler.staticExpressions = append(compiler.staticExpressions, StaticExpressionRow{id: expressionID, reference: nodeID, owner: owner})
	}
	for inputIndex := 0; inputIndex < typeOfs.Count(); inputIndex++ {
		sourceTerm, inputOK := typeOfs.At(inputIndex)
		if inputOK {
			_, operandTerm, inputOK := typeOfs.Get(sourceTerm)
			if inputOK {
				frontierID, cursor, frontierOK := ownerProgram.StaticFrontier(sourceTerm)
				expressionRef, expressionOK := view.StaticTypes().Ref(sourceTerm)
				sourceID, sourceOK := staticNodeID(owner, expressionRef)
				operandID, operandOK := programstatic.OccurrenceID(owner, 1, operandTerm)
				operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
				if semanticOperandID.Available() {
					operandID = semanticOperandID
				}
				if !expressionOK || !sourceOK || !operandOK || !frontierOK || !dispositionOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				rowID, rowIDOK := programstatic.InputID(owner, 2, sourceTerm, uint32(inputIndex))
				if !rowIDOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				// The expression row is created below from the same canonical
				// Static reference, so it must already be present here.
				expressionID, expressionIDOK := expressionIDs[sourceTerm]
				if !expressionIDOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				compiler.staticInputs = append(compiler.staticInputs, StaticInputRow{id: rowID, owner: owner, expression: expressionID, source: sourceID, target: sourceID, operand: operandID, frontier: frontierID, kind: StaticInputTypeOf, operandKind: operandKind, literal: literal, operandReference: operandReference, operandSubject: operandSubject, operandBody: operandBody, cursor: cursor})
				continue
			}
		}
		return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
	}
	for annotationIndex := 0; annotationIndex < annotations.Count(); annotationIndex++ {
		sourceTerm, annotationOK := annotations.At(annotationIndex)
		var targetTerm, valuesTerm keyspace.Term
		if annotationOK {
			annotation, ok := annotations.Get(sourceTerm)
			annotationOK = ok
			if ok {
				targetTerm, valuesTerm = annotation.Target, annotation.Values
			}
		}
		if !annotationOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		count, countOK := ownerProgram.Flow().Authored().Values().Len(valuesTerm)
		if !countOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		targetRef, targetRefOK := view.StaticTypes().Ref(targetTerm)
		targetID, targetIDOK := staticNodeID(owner, targetRef)
		frontierID, cursor, frontierOK := ownerProgram.StaticFrontier(sourceTerm)
		if !targetRefOK || !targetIDOK || !frontierOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		annotationSourceID, annotationSourceOK := programstatic.OccurrenceID(owner, 5, sourceTerm)
		if !annotationSourceOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		for valueIndex := 0; valueIndex < count; valueIndex++ {
			operandTerm, operandOK := ownerProgram.Flow().Authored().Values().Member(valuesTerm, valueIndex)
			operandID, operandIDOK := programstatic.OccurrenceID(owner, 3, operandTerm)
			operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
			if semanticOperandID.Available() {
				operandID = semanticOperandID
			}
			rowID, rowIDOK := programstatic.InputID(owner, 5, sourceTerm, uint32(valueIndex))
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
		childID := func(child keyspace.Term) (identity.ContentID, bool) {
			childRef, childRefOK := view.StaticTypes().Ref(child)
			if !childRefOK {
				return identity.ContentID{}, false
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
			row.exact, _ = ownerProgram.Source().Keys().Exact(key)
			nameLiteral, nameOK := ownerProgram.Source().Keys().Exact(key)
			if nameOK && nameLiteral.Kind == keyspace.LiteralString {
				row.name = nameLiteral.String
			}
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
					textLiteral, textOK := ownerProgram.Source().Keys().Exact(fieldKey)
					text := ""
					if textOK && textLiteral.Kind == keyspace.LiteralString {
						text = textLiteral.String
					}
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
			// Unresolved reference observations are built once by the artifact
			// diagnostic column builder from the canonical Static view. Static
			// graph admission only records the reference node itself.
		} else if _, target, nameKey, _, found := view.Declarations().Aliases().Get(term); found {
			row.kind = StaticNodeAlias
			row.key = nameKey
			nameLiteral, nameOK := ownerProgram.Source().Keys().Exact(nameKey)
			if nameOK && nameLiteral.Kind == keyspace.LiteralString {
				row.name = nameLiteral.String
			}
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
			nameLiteral, nameOK := ownerProgram.Source().Keys().Exact(nameKey)
			if nameOK && nameLiteral.Kind == keyspace.LiteralString {
				row.name = nameLiteral.String
			}
			if declOwner != 0 {
				row.declaration, ok = programstatic.ScopeID(ownerIDForRow, declOwner)
			}
			if constraint != 0 && ok {
				ok = appendChild(constraint)
			}
		} else if interfaceOwner, nameKey, _, found := view.Declarations().Interfaces().Get(term); found {
			row.kind = StaticNodeInterface
			row.key = nameKey
			nameLiteral, nameOK := ownerProgram.Source().Keys().Exact(nameKey)
			if nameOK && nameLiteral.Kind == keyspace.LiteralString {
				row.name = nameLiteral.String
			}
			if interfaceOwner != 0 {
				row.declaration, ok = programstatic.ScopeID(ownerIDForRow, interfaceOwner)
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
					memberLiteral, memberOK := ownerProgram.Source().Keys().Exact(memberKey)
					memberText := ""
					if memberOK && memberLiteral.Kind == keyspace.LiteralString {
						memberText = memberLiteral.String
					}
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
			row.scope, ok = programstatic.ScopeID(owner, scope)
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
					parameterLiteral, parameterOK := ownerProgram.Source().Keys().Exact(parameter.Name)
					parameterText := ""
					if parameterOK && parameterLiteral.Kind == keyspace.LiteralString {
						parameterText = parameterLiteral.String
					}
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
			row.operand, ok = programstatic.OccurrenceID(owner, 1, operand)
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
			nameLiteral, nameOK := ownerProgram.Source().Keys().Exact(nameKey)
			if nameOK && nameLiteral.Kind == keyspace.LiteralString {
				row.name = nameLiteral.String
			}
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
