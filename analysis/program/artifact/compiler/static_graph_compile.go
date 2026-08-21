package compiler

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	staticdecl "github.com/wippyai/go-lua/analysis/program/static/declarations"
	staticquery "github.com/wippyai/go-lua/analysis/program/static/query"
	programschema "github.com/wippyai/go-lua/analysis/schema/program"
	programfamily "github.com/wippyai/go-lua/analysis/schema/program/family"
	staticnode "github.com/wippyai/go-lua/analysis/schema/program/staticnode"
)

func staticNodeID(owner identity.ContentID, ref staticquery.StaticTypeRef) (id identity.ContentID, ok bool) {
	if !owner.Available() || ref.Term() == 0 {
		return identity.ContentID{}, false
	}
	return staticquery.TypeReferenceID(owner, ref)
}

type staticNodeChildConstructor[V programfamily.Row] func(identity.ContentID, identity.ContentID, uint32) (V, bool)

// appendStaticNodeChild emits one typed relation row. All static child
// families share this construction rule; the constructor and destination keep
// their schemas distinct without repeating row-transport ceremony.
func appendStaticNodeChild[V programfamily.Row](rows *[]V, parent identity.ContentID, term keyspace.Term, position int, childID func(keyspace.Term) (identity.ContentID, bool), construct staticNodeChildConstructor[V]) bool {
	if rows == nil || childID == nil || construct == nil || position < 0 || uint64(position) > uint64(^uint32(0)) {
		return false
	}
	child, ok := childID(term)
	if !ok {
		return false
	}
	row, ok := construct(parent, child, uint32(position))
	if !ok {
		return false
	}
	*rows = append(*rows, row)
	return true
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
	programID := compiler.key.ProgramID()
	rows := make([]staticnode.StaticTypeNode, 0, view.StaticTypes().Count())
	compiler.staticExpressions = make([]programschema.StaticExpression, 0, view.StaticTypes().Count())
	typeOfs := view.Operators().TypeOfs()
	annotations := view.Operands().Annotations()
	compiler.staticInputs = make([]programschema.StaticInput, 0, typeOfs.Count())
	operandRow := func(term keyspace.Term) (staticquery.StaticOperandKind, keyspace.LiteralValue, identity.ContentID, identity.ContentID, identity.ContentID, identity.ContentID, bool) {
		operand, ok := artifactStaticOperandAt(ownerProgram, programID, term)
		if !ok {
			return staticquery.StaticOperandInvalid, keyspace.LiteralValue{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
		}
		var kind staticquery.StaticOperandKind
		switch operand.Kind() {
		case staticquery.StaticOperandKnown:
			kind = staticquery.StaticOperandKnown
		case staticquery.StaticOperandRuntimeSubject:
			kind = staticquery.StaticOperandRuntimeSubject
		case staticquery.StaticOperandTypeValue:
			kind = staticquery.StaticOperandTypeValue
		default:
			return staticquery.StaticOperandInvalid, keyspace.LiteralValue{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, identity.ContentID{}, false
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
		expressionID, expressionOK := staticquery.ExpressionID(owner, ref)
		if !nodeOK || !expressionOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		expressionIDs[ref.Term()] = expressionID
		expressionRow, expressionRowOK := programschema.NewStaticExpression(expressionID, nodeID, owner)
		if !expressionRowOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, expressionIndex, -1, CompileReasonProgramUnavailable)
		}
		compiler.staticExpressions = append(compiler.staticExpressions, expressionRow)
	}
	for inputIndex := 0; inputIndex < typeOfs.Count(); inputIndex++ {
		sourceTerm, inputOK := typeOfs.At(inputIndex)
		if inputOK {
			_, operandTerm, inputOK := typeOfs.Get(sourceTerm)
			if inputOK {
				frontierID, cursor, frontierOK := artifactStaticFrontier(ownerProgram, sourceTerm)
				expressionRef, expressionOK := view.StaticTypes().Ref(sourceTerm)
				sourceID, sourceOK := staticNodeID(owner, expressionRef)
				operandID, operandOK := staticquery.OccurrenceID(owner, 1, operandTerm)
				operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
				if semanticOperandID.Available() {
					operandID = semanticOperandID
				}
				if !expressionOK || !sourceOK || !operandOK || !frontierOK || !dispositionOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				rowID, rowIDOK := staticquery.InputID(owner, 2, sourceTerm, uint32(inputIndex))
				if !rowIDOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				// The expression row is created below from the same canonical
				// Static reference, so it must already be present here.
				expressionID, expressionIDOK := expressionIDs[sourceTerm]
				if !expressionIDOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				row, rowOK := programschema.NewStaticInput(rowID, owner, expressionID, sourceID, sourceID, operandID, frontierID, operandReference, operandSubject, operandBody, literal, programschema.StaticInputTypeOf, uint8(operandKind), cursor)
				if !rowOK {
					return compileFailure(CompileStageAuthority, CompileRowAuthority, inputIndex, -1, CompileReasonProgramUnavailable)
				}
				compiler.staticInputs = append(compiler.staticInputs, row)
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
		frontierID, cursor, frontierOK := artifactStaticFrontier(ownerProgram, sourceTerm)
		if !targetRefOK || !targetIDOK || !frontierOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		annotationSourceID, annotationSourceOK := staticquery.OccurrenceID(owner, 5, sourceTerm)
		if !annotationSourceOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, -1, CompileReasonProgramUnavailable)
		}
		for valueIndex := 0; valueIndex < count; valueIndex++ {
			operandTerm, operandOK := ownerProgram.Flow().Authored().Values().Member(valuesTerm, valueIndex)
			operandID, operandIDOK := staticquery.OccurrenceID(owner, 3, operandTerm)
			operandKind, literal, semanticOperandID, operandReference, operandSubject, operandBody, dispositionOK := operandRow(operandTerm)
			if semanticOperandID.Available() {
				operandID = semanticOperandID
			}
			rowID, rowIDOK := staticquery.InputID(owner, 5, sourceTerm, uint32(valueIndex))
			if !operandOK || !operandIDOK || !rowIDOK || !dispositionOK {
				return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, valueIndex, CompileReasonProgramUnavailable)
			}
			expressionID, expressionIDOK := expressionIDs[targetTerm]
			if !expressionIDOK {
				return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, valueIndex, CompileReasonProgramUnavailable)
			}
			row, rowOK := programschema.NewStaticInput(rowID, owner, expressionID, annotationSourceID, targetID, operandID, frontierID, operandReference, operandSubject, operandBody, literal, programschema.StaticInputAnnotation, uint8(operandKind), cursor)
			if !rowOK {
				return compileFailure(CompileStageAuthority, CompileRowAuthority, annotationIndex, valueIndex, CompileReasonProgramUnavailable)
			}
			compiler.staticInputs = append(compiler.staticInputs, row)
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
		spec := staticnode.StaticTypeNodeSpec{ID: id, Owner: owner, Kind: staticnode.StaticNodeUnknown}
		childID := func(child keyspace.Term) (identity.ContentID, bool) {
			childRef, childRefOK := view.StaticTypes().Ref(child)
			if !childRefOK {
				return identity.ContentID{}, false
			}
			return staticNodeID(owner, childRef)
		}
		textForKey := func(key keyspace.Key) string {
			literal, found := ownerProgram.Source().Keys().Exact(key)
			if found && literal.Kind == keyspace.LiteralString {
				return literal.String
			}
			return ""
		}
		ok = true
		if primitive, found := view.Types().Primitives().Get(term); found {
			spec.Kind, spec.Literal = staticnode.StaticNodePrimitive, uint8(primitive)
		} else if literal, key, bits, found := view.Types().Literals().Get(term); found {
			spec.Kind, spec.Literal, spec.Key, spec.Bits = staticnode.StaticNodeLiteral, uint8(literal), key, bits
			spec.Exact, _ = ownerProgram.Source().Keys().Exact(key)
			spec.Name = textForKey(key)
		} else if child, found := view.Types().Optionals().Get(term); found {
			spec.Kind = staticnode.StaticNodeOptional
			spec.OptionalInner, ok = childID(child)
		} else if count, found := view.Types().Unions().MemberCount(term); found {
			spec.Kind, spec.UnionOffset = staticnode.StaticNodeUnion, uint32(len(compiler.staticTypeNodeUnionMembers))
			spec.UnionCount = uint32(count)
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Unions().MemberAt(term, n)
				ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeUnionMembers, id, child, n, childID, staticnode.NewStaticTypeNodeUnionMember)
			}
		} else if count, found := view.Types().Intersections().MemberCount(term); found {
			spec.Kind, spec.IntersectionOffset = staticnode.StaticNodeIntersection, uint32(len(compiler.staticTypeNodeIntersectionMembers))
			spec.IntersectionCount = uint32(count)
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Intersections().MemberAt(term, n)
				ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeIntersectionMembers, id, child, n, childID, staticnode.NewStaticTypeNodeIntersectionMember)
			}
		} else if base, count, found := view.Types().Generics().Get(term); found {
			spec.Kind, spec.GenericBase, spec.GenericArgumentOffset = staticnode.StaticNodeGeneric, identity.ContentID{}, uint32(len(compiler.staticTypeNodeGenericArguments))
			spec.GenericArgumentCount = uint32(count)
			spec.GenericBase, ok = childID(base)
			for n := 0; n < count && ok; n++ {
				child, childOK := view.Types().Generics().ArgAt(term, n)
				ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeGenericArguments, id, child, n, childID, staticnode.NewStaticTypeNodeGenericArgument)
			}
		} else if child, readonly, found := view.Types().Arrays().Get(term); found {
			spec.Kind, spec.Flag = staticnode.StaticNodeArray, readonly
			spec.ArrayElement, ok = childID(child)
		} else if key, value, readonly, found := view.Types().Maps().Get(term); found {
			spec.Kind, spec.Flag = staticnode.StaticNodeMap, readonly
			spec.MapKey, ok = childID(key)
			if ok {
				spec.MapValue, ok = childID(value)
			}
		} else if readonly, count, found := view.Types().Records().Get(term); found {
			spec.Kind, spec.Flag, spec.RecordFieldOffset, spec.RecordFieldCount = staticnode.StaticNodeRecord, readonly, uint32(len(compiler.staticTypeNodeRecordFields)), uint32(count)
			for n := 0; n < count && ok; n++ {
				field, fieldOK := view.Types().Records().FieldAt(term, n)
				fieldKey, fieldType, optional, fieldShapeOK := view.Types().Fields().Get(field)
				if !fieldOK || !fieldShapeOK {
					ok = false
					break
				}
				fieldID, childOK := childID(fieldType)
				text := textForKey(fieldKey)
				ok = childOK
				if ok {
					member, memberOK := staticnode.NewStaticTypeNodeRecordField(id, fieldID, fieldKey, text, optional, readonly, uint32(n))
					ok = memberOK
					if ok {
						compiler.staticTypeNodeRecordFields = append(compiler.staticTypeNodeRecordFields, member)
					}
				}
			}
		} else if resolution, target, _, found := view.References().Get(term); found {
			spec.Kind, spec.Resolution = staticnode.StaticNodeReference, uint8(resolution)
			if count, countOK := view.References().SourceCount(term); countOK {
				spec.ReferenceSourceKeyOffset, spec.ReferenceSourceKeyCount = uint32(len(compiler.staticTypeNodeReferenceSourceKeys)), uint32(count)
				for n := 0; n < count && ok; n++ {
					key, keyOK := view.References().SourceAt(term, n)
					if !keyOK {
						ok = false
						break
					}
					member, memberOK := staticnode.NewStaticTypeNodeReferenceSourceKey(id, key, uint32(n))
					ok = memberOK
					if ok {
						compiler.staticTypeNodeReferenceSourceKeys = append(compiler.staticTypeNodeReferenceSourceKeys, member)
					}
				}
			}
			if count, countOK := view.References().CanonicalCount(term); countOK {
				spec.ReferenceCanonicalKeyOffset, spec.ReferenceCanonicalKeyCount = uint32(len(compiler.staticTypeNodeReferenceCanonicalKeys)), uint32(count)
				for n := 0; n < count && ok; n++ {
					key, keyOK := view.References().CanonicalAt(term, n)
					if !keyOK {
						ok = false
						break
					}
					member, memberOK := staticnode.NewStaticTypeNodeReferenceCanonicalKey(id, key, uint32(n))
					ok = memberOK
					if ok {
						compiler.staticTypeNodeReferenceCanonicalKeys = append(compiler.staticTypeNodeReferenceCanonicalKeys, member)
					}
				}
			}
			if target != 0 {
				spec.ReferenceTarget, ok = childID(target)
			}
		} else if _, target, nameKey, _, found := view.Declarations().Aliases().Get(term); found {
			spec.Kind, spec.Key, spec.Name = staticnode.StaticNodeAlias, nameKey, textForKey(nameKey)
			spec.AliasTarget, ok = childID(target)
			if count, countOK := view.Declarations().Aliases().ParamCount(term); countOK {
				spec.SegmentCount, spec.Segments[0], spec.AliasParameterOffset, spec.AliasParameterCount = 1, uint32(count), uint32(len(compiler.staticTypeNodeAliasParameters)), uint32(count)
				for n := 0; n < count && ok; n++ {
					param, paramOK := view.Declarations().Aliases().ParamAt(term, n)
					ok = paramOK && appendStaticNodeChild(&compiler.staticTypeNodeAliasParameters, id, param, n, childID, staticnode.NewStaticTypeNodeAliasParameter)
				}
			}
		} else if declOwner, nameKey, constraint, found := view.Declarations().TypeParams().Get(term); found {
			spec.Kind, spec.Key, spec.Name = staticnode.StaticNodeTypeParam, nameKey, textForKey(nameKey)
			if declOwner != 0 {
				spec.Declaration, ok = staticquery.ScopeID(ownerIDForRow, declOwner)
			}
			if constraint != 0 && ok {
				spec.TypeParamConstraint, ok = childID(constraint)
			}
		} else if interfaceOwner, nameKey, _, found := view.Declarations().Interfaces().Get(term); found {
			spec.Kind, spec.Key, spec.Name = staticnode.StaticNodeInterface, nameKey, textForKey(nameKey)
			if interfaceOwner != 0 {
				spec.Declaration, ok = staticquery.ScopeID(ownerIDForRow, interfaceOwner)
			}
			if count, shapeOK := view.Declarations().Interfaces().ExtendCount(term); shapeOK {
				spec.Segments[spec.SegmentCount], spec.SegmentCount, spec.InterfaceExtendOffset, spec.InterfaceExtendCount = uint32(count), spec.SegmentCount+1, uint32(len(compiler.staticTypeNodeInterfaceExtends)), uint32(count)
				for n := 0; n < count && ok; n++ {
					child, childOK := view.Declarations().Interfaces().ExtendAt(term, n)
					ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeInterfaceExtends, id, child, n, childID, staticnode.NewStaticTypeNodeInterfaceExtend)
				}
			}
			if count, shapeOK := view.Declarations().Interfaces().MemberCount(term); shapeOK {
				spec.Segments[spec.SegmentCount], spec.SegmentCount, spec.InterfaceMemberOffset, spec.InterfaceMemberCount = uint32(count), spec.SegmentCount+1, uint32(len(compiler.staticTypeNodeInterfaceMembers)), uint32(count)
				for n := 0; n < count && ok; n++ {
					member, memberOK := view.Declarations().Interfaces().MemberAt(term, n)
					if !memberOK {
						ok = false
						break
					}
					memberKey, memberType, memberOptional := member.Name, member.Signature, false
					if member.Kind == staticdecl.InterfaceField && member.Field != 0 {
						fieldKey, fieldType, optional, fieldOK := view.Types().Fields().Get(member.Field)
						if !fieldOK {
							ok = false
							break
						}
						memberKey, memberType, memberOptional = fieldKey, fieldType, optional
					}
					memberID, idOK := childID(memberType)
					if !idOK {
						ok = false
						break
					}
					typed, typedOK := staticnode.NewStaticTypeNodeInterfaceMember(id, memberID, memberKey, textForKey(memberKey), memberOptional, false, uint8(member.Kind), uint32(n))
					ok = typedOK
					if ok {
						compiler.staticTypeNodeInterfaceMembers = append(compiler.staticTypeNodeInterfaceMembers, typed)
					}
				}
			}
		} else if scope, variadic, _, returnsKnown, found := view.Signatures().TypeFunctions().Get(term); found {
			spec.Kind, spec.Flag, spec.ReturnsKnown = staticnode.StaticNodeTypeFunction, variadic != 0, returnsKnown
			spec.Scope, ok = staticquery.ScopeID(owner, scope)
			if variadic != 0 {
				spec.TypeFunctionVariadic, ok = childID(variadic)
				spec.Segments[spec.SegmentCount], spec.SegmentCount = 1, spec.SegmentCount+1
			} else {
				spec.Segments[spec.SegmentCount], spec.SegmentCount = 0, spec.SegmentCount+1
			}
			if count, shapeOK := view.Signatures().TypeFunctions().TypeParamCount(term); shapeOK {
				spec.Segments[spec.SegmentCount], spec.SegmentCount, spec.TypeFunctionTypeParameterOffset, spec.TypeFunctionTypeParameterCount = uint32(count), spec.SegmentCount+1, uint32(len(compiler.staticTypeNodeTypeFunctionTypeParameters)), uint32(count)
				for n := 0; n < count && ok; n++ {
					child, childOK := view.Signatures().TypeFunctions().TypeParamAt(term, n)
					ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeTypeFunctionTypeParameters, id, child, n, childID, staticnode.NewStaticTypeNodeTypeFunctionTypeParameter)
				}
			}
			if count, shapeOK := view.Signatures().TypeFunctions().ParameterCount(term); shapeOK {
				spec.Segments[spec.SegmentCount], spec.SegmentCount, spec.TypeFunctionParameterOffset, spec.TypeFunctionParameterCount = uint32(count), spec.SegmentCount+1, uint32(len(compiler.staticTypeNodeTypeFunctionParameters)), uint32(count)
				for n := 0; n < count && ok; n++ {
					parameter, parameterOK := view.Signatures().TypeFunctions().ParameterAt(term, n)
					if !parameterOK {
						ok = false
						break
					}
					parameterID, idOK := childID(parameter.Type)
					if !idOK {
						ok = false
						break
					}
					typed, typedOK := staticnode.NewStaticTypeNodeTypeFunctionParameter(id, parameterID, parameter.Name, textForKey(parameter.Name), uint32(n))
					ok = typedOK
					if ok {
						compiler.staticTypeNodeTypeFunctionParameters = append(compiler.staticTypeNodeTypeFunctionParameters, typed)
					}
				}
			}
			if returnsKnown {
				if count, shapeOK := view.Signatures().TypeFunctions().ReturnCount(term); shapeOK {
					spec.Segments[spec.SegmentCount], spec.SegmentCount, spec.TypeFunctionReturnOffset, spec.TypeFunctionReturnCount = uint32(count), spec.SegmentCount+1, uint32(len(compiler.staticTypeNodeTypeFunctionReturns)), uint32(count)
					for n := 0; n < count && ok; n++ {
						child, childOK := view.Signatures().TypeFunctions().ReturnAt(term, n)
						ok = childOK && appendStaticNodeChild(&compiler.staticTypeNodeTypeFunctionReturns, id, child, n, childID, staticnode.NewStaticTypeNodeTypeFunctionReturn)
					}
				}
			}
		} else if _, operand, found := view.Operators().TypeOfs().Get(term); found {
			spec.Kind = staticnode.StaticNodeTypeOf
			ok = false
			spec.Operand, ok = staticquery.OccurrenceID(owner, 1, operand)
		} else if child, found := view.Operators().KeyOfs().Get(term); found {
			spec.Kind = staticnode.StaticNodeKeyOf
			ok = false
			spec.KeyOfChild, ok = childID(child)
		} else if object, indexTerm, found := view.Operators().IndexAccesses().Get(term); found {
			spec.Kind = staticnode.StaticNodeIndex
			ok = false
			spec.IndexObject, ok = childID(object)
			if ok {
				spec.IndexKey, ok = childID(indexTerm)
			}
		} else if check, extends, thenTerm, otherwise, found := view.Operators().Conditionals().Get(term); found {
			spec.Kind = staticnode.StaticNodeConditional
			spec.ConditionalCheck, ok = childID(check)
			if ok {
				spec.ConditionalExtends, ok = childID(extends)
			}
			if ok {
				spec.ConditionalThen, ok = childID(thenTerm)
			}
			if ok {
				spec.ConditionalOtherwise, ok = childID(otherwise)
			}
		} else if nameKey, coordinate, bound, param, narrow, found := view.Signatures().Assertions().Get(term); found {
			spec.Kind, spec.Key, spec.Name, spec.Flag, spec.AssertParam = staticnode.StaticNodeAssertion, nameKey, textForKey(nameKey), bound, param
			if narrow != 0 {
				spec.AssertionNarrow, ok = childID(narrow)
			}
			spec.AssertionCoordinate[0], spec.AssertionCoordinate[1], spec.AssertionCoordinate[2], spec.AssertionCoordinate[3] = coordinate.Parts()
		}
		if spec.Kind == staticnode.StaticNodeUnknown || !ok {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, index, -1, CompileReasonProgramUnavailable)
		}
		node, nodeOK := staticnode.NewStaticTypeNode(spec)
		if !nodeOK {
			return compileFailure(CompileStageAuthority, CompileRowAuthority, index, -1, CompileReasonProgramUnavailable)
		}
		rows = append(rows, node)
	}
	compiler.staticTypeNodes = rows
	return CompileFailure{}
}
