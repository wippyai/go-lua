package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func artifactID(artifact *Artifact) identity.ContentID {
	sink := newDigestSink(artifactIDDomain, artifactFormat)
	sink.add(bytesField(artifact.key.ID()))
	sink.add(artifact.key.identityFields()...)
	sink.add(uintField(uint64(artifact.counts.Count())))
	for index := 0; index < artifact.counts.Count(); index++ {
		row, ok := artifact.counts.At(index)
		if !ok {
			return identity.ContentID{}
		}
		sink.add(bytesField(identity.ContentID(row.ID())), uintField(row.Count()))
	}
	sink.add(uintField(pointGeometryLawVersion))
	sink.add(uintField(pointAttachmentLawVersion))
	sink.add(uintField(uint64(len(artifact.points))))
	for _, point := range artifact.points {
		sink.add(bytesField(point.id), boolField(point.initial), uintField(uint64(len(point.decisions))))
		for _, decision := range point.decisions {
			sink.add(bytesField(decision))
		}
	}
	// Values and its member plane are read out of the sealed cold publication.
	// The member span preserves the emitted order, so the preimage is the same
	// sequence the identity has always committed to.
	valuesCount, valuesPublished := coldCount(artifact, cold.ValuesFamily())
	if !valuesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(valuesCount)))
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, cold.ValuesFamily(), index)
		offset, members, spanOK := row.MemberSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.BodyPathID()), uintField(uint64(members)))
		for position := uint32(0); position < members; position++ {
			member, memberHeld := coldRow(artifact, cold.ValuesMemberFamily(), int(offset+position))
			if !memberHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(member.ID()))
		}
		tail, present := row.Tail()
		sink.add(boolField(present), uintField(uint64(tail.Kind())), bytesField(tail.ID()))
	}
	// Calls are an Artifact-owned source column. Keep
	// every scalar and ordered child identity in the artifact seal so replay,
	// mutation, and mounted joins authenticate the direct plane exactly.
	sink.add(uintField(callRowsLawVersion), uintField(uint64(len(artifact.calls))))
	for _, row := range artifact.calls {
		sink.add(
			bytesField(row.id), bytesField(row.body), bytesField(row.span), bytesField(row.formal),
			bytesField(row.values), bytesField(row.valuesRoot), bytesField(row.types), bytesField(row.callee), bytesField(row.actuals),
			bytesField(row.target),
			uintField(uint64(row.form)), boolField(row.hasReceiver), bytesField(row.receiver), boolField(row.hasTail), bytesField(row.tail),
			uintField(uint64(row.OperandCount())), uintField(uint64(row.ArgumentCount())), uintField(uint64(row.TypeArgumentCount())),
		)
		for index := int(row.operandStart); index < int(row.operandEnd); index++ {
			operand := artifact.callOperands[index]
			sink.add(bytesField(operand.id), bytesField(operand.call), bytesField(operand.value), bytesField(operand.span), uintField(uint64(operand.kind)))
		}
		for index := int(row.argumentStart); index < int(row.argumentEnd); index++ {
			argument := artifact.callArguments[index]
			sink.add(bytesField(argument.id), bytesField(argument.call), bytesField(argument.values), bytesField(argument.member), bytesField(argument.span), uintField(uint64(argument.position)))
		}
		for index := int(row.typeArgumentStart); index < int(row.typeArgumentEnd); index++ {
			argument := artifact.callTypeArguments[index]
			sink.add(bytesField(argument.id), bytesField(argument.call), bytesField(argument.types), bytesField(argument.reference), uintField(uint64(argument.position)))
		}
	}
	sink.add(uintField(uint64(len(artifact.bodies))))
	for _, body := range artifact.bodies {
		sink.add(bytesField(body.id), bytesField(body.context), bytesField(body.entry), boolField(body.callable), bytesField(body.function), bytesField(body.formal), uintField(uint64(len(body.entryPoints))))
		for _, point := range body.entryPoints {
			sink.add(bytesField(point))
		}
		sink.add(uintField(uint64(len(body.roots))))
		for _, root := range body.roots {
			sink.add(bytesField(root.id), uintField(uint64(root.family)))
		}
		sink.add(uintField(uint64(body.outcomeStart)), uintField(uint64(body.outcomeEnd)))
	}
	sink.add(uintField(functionBoundaryLawVersion), uintField(uint64(len(artifact.functionBoundaries))))
	for _, boundary := range artifact.functionBoundaries {
		sink.add(
			bytesField(boundary.id), bytesField(boundary.body), bytesField(boundary.bodyContext), bytesField(boundary.entry), bytesField(boundary.callFormal),
			uintField(uint64(len(boundary.formals))),
		)
		for _, port := range boundary.formals {
			sink.add(bytesField(port.id), bytesField(port.cell), bytesField(port.storage), bytesField(port.declared), uintField(uint64(port.position)))
		}
		sink.add(boolField(boundary.hasVararg), bytesField(boundary.vararg.id), bytesField(boundary.vararg.cell), uintField(uint64(len(boundary.captures))))
		for _, capture := range boundary.captures {
			sink.add(
				bytesField(capture.id), bytesField(capture.inner), bytesField(capture.outer), bytesField(capture.innerBody), bytesField(capture.outerBody), uintField(uint64(capture.position)),
			)
		}
	}
	// The call-target family is read out of the sealed cold publication in its
	// emitted order, which is the order the identity has always committed to.
	targets, published := cold.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !published {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(targets)))
	for index := 0; index < targets; index++ {
		target, held := cold.CallTargetFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(bytesField(target.Allocation), bytesField(target.Body), bytesField(target.Context), bytesField(target.Function), bytesField(target.Formal))
	}
	sink.add(uintField(uint64(len(artifact.outcomes))))
	for _, outcome := range artifact.outcomes {
		sink.add(
			bytesField(outcome.id), bytesField(outcome.body), uintField(uint64(outcome.kind)),
			boolField(outcome.hasTarget), bytesField(outcome.target),
			boolField(outcome.hasPropagation), bytesField(outcome.propagation),
			uintField(uint64(outcome.returnStart)), uintField(uint64(outcome.returnEnd)), uintField(uint64(len(outcome.points))),
		)
		for _, point := range outcome.points {
			sink.add(bytesField(point))
		}
	}
	sink.add(uintField(uint64(len(artifact.returnValues))))
	for _, value := range artifact.returnValues {
		sink.add(bytesField(value.id))
	}
	sink.add(uintField(uint64(len(artifact.occurrences))))
	for _, row := range artifact.occurrences {
		sink.add(uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.body), uintField(row.code), uintField(uint64(len(row.points))))
		for _, point := range row.points {
			sink.add(bytesField(point))
		}
		sink.add(uintField(uint64(len(row.inputs))))
		for _, input := range row.inputs {
			sink.add(bytesField(input))
		}
		sink.add(uintField(uint64(row.literalFamily)), boolField(row.literalOK), uintField(uint64(row.literal.Kind)), boolField(row.literal.Bool), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits), field{bytes: []byte(row.literal.String), kind: fieldBytes})
	}
	exactCount, exactPublished := cold.ExactScalarSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !exactPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(exactCount)))
	for index := 0; index < exactCount; index++ {
		row, rowOK := cold.ExactScalarSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		literal, literalOK := row.Literal()
		if !rowOK || !literalOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.SubjectID()), bytesField(row.BodyPathID()),
			uintField(uint64(row.Role())), uintField(uint64(literal.Kind)), uintField(uint64(literal.Integer)), uintField(literal.FloatBits))
	}
	arithmeticCount, arithmeticPublished := cold.ArithmeticSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !arithmeticPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(arithmeticCount)))
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := cold.ArithmeticSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		left, right, result, representationsOK := row.Representations()
		if !rowOK || !representationsOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.BodyPathID()), uintField(uint64(row.Operator())),
			uintField(uint64(left)), uintField(uint64(right)), uintField(uint64(result)), uintField(uint64(row.DivisorProperty())))
	}
	unaryCount, unaryPublished := cold.UnarySummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !unaryPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(unaryCount)))
	for index := 0; index < unaryCount; index++ {
		row, rowOK := cold.UnarySummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		operand, result, representationsOK := row.Representations()
		if !rowOK || !representationsOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.BodyPathID()), bytesField(row.OutputPointID()), uintField(uint64(row.Operator())),
			uintField(uint64(operand)), uintField(uint64(result)))
	}
	allocationCount, allocationsPublished := coldCount(artifact, cold.HeapAllocationFamily())
	if !allocationsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(allocationCount)))
	for index := 0; index < allocationCount; index++ {
		allocation, held := coldRow(artifact, cold.HeapAllocationFamily(), index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(allocation.ID()), uintField(uint64(allocation.Role())), uintField(uint64(allocation.Form())), bytesField(allocation.RootSpan()), uintField(uint64(fields)))
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := coldRow(artifact, cold.HeapFieldFamily(), int(offset+position))
			valuesSpan, width, finalOpen, valuesOK := field.Values()
			normalized, normalizedOK := field.NormalizedKey()
			if !fieldHeld || !valuesOK {
				return identity.ContentID{}
			}
			sink.add(bytesField(field.ID()), uintField(uint64(field.Kind())), bytesField(field.FieldSpan()), bytesField(field.SelectorSpan()), bytesField(valuesSpan), bytesField(field.ValuesID()), uintField(uint64(width)), boolField(finalOpen), boolField(field.SharesFirstValueCell()), uintField(normalized), boolField(normalizedOK))
		}
	}
	indexCount, indexesPublished := coldCount(artifact, cold.HeapIndexFamily())
	if !indexesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(indexCount)))
	for index := 0; index < indexCount; index++ {
		access, held := coldRow(artifact, cold.HeapIndexFamily(), index)
		if !held {
			return identity.ContentID{}
		}
		exactKey, _ := access.ExactKey()
		valuesSpan, _, _ := access.Values()
		sink.add(bytesField(access.ID()), boolField(access.Read()), bytesField(access.BaseSpan()), bytesField(access.ResultSpan()), bytesField(access.DynamicKeySpan()), uintField(uint64(access.LensKind())), uintField(exactKey), bytesField(valuesSpan), bytesField(access.ValuesID()), uintField(uint64(access.Position()+1)))
	}
	sink.add(uintField(diagnosticLawVersion), uintField(uint64(len(artifact.diagnosticObservations))))
	for _, row := range artifact.diagnosticObservations {
		sink.add(
			bytesField(row.id), uintField(uint64(row.kind)),
			field{bytes: []byte(row.location.File), kind: fieldBytes}, uintField(uint64(row.location.StartLine)),
			uintField(uint64(row.location.StartCol)), uintField(uint64(row.location.EndLine)), uintField(uint64(row.location.EndCol)),
		)
		switch row.kind {
		case structure.DiagnosticObservationBranchCondition:
			sink.add(bytesField(row.branch.decision), bytesField(row.branch.value), uintField(uint64(len(row.branch.points))))
			for _, point := range row.branch.points {
				sink.add(bytesField(point))
			}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			sink.add(bytesField(row.unresolved.reference), bytesField(row.unresolved.root), uintField(uint64(len(row.unresolved.path))))
			for _, component := range row.unresolved.path {
				sink.add(field{bytes: []byte(component), kind: fieldBytes})
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			sink.add(bytesField(row.value.read), bytesField(row.value.cell), field{bytes: []byte(row.value.name), kind: fieldBytes})
		case structure.DiagnosticObservationTypeConformance:
			sink.add(
				uintField(uint64(row.conformance.site)), bytesField(row.conformance.call), bytesField(row.conformance.argument),
				bytesField(row.conformance.declared), bytesField(row.conformance.span), uintField(uint64(row.conformance.position)),
				uintField(uint64(len(row.conformance.points))),
			)
			for _, point := range row.conformance.points {
				sink.add(bytesField(point))
			}
		default:
			// An observation kind this walk does not carry would contribute
			// only its header, so two rows differing in payload would share
			// one identity. The seal refuses instead.
			return identity.ContentID{}
		}
	}
	sink.add(uintField(uint64(len(artifact.staticTypeValues))))
	for _, row := range artifact.staticTypeValues {
		sink.add(bytesField(row.id), bytesField(row.body), bytesField(row.reference), bytesField(row.root), field{bytes: []byte(row.name), kind: fieldBytes})
	}
	sink.add(uintField(uint64(len(artifact.staticTypeNodes))))
	for _, row := range artifact.staticTypeNodes {
		exact := row.exact
		sink.add(bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), field{bytes: []byte(row.name), kind: fieldBytes}, uintField(uint64(row.key)), uintField(uint64(row.literal)), uintField(row.bits), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, boolField(row.flag), uintField(uint64(row.resolution)), uintField(uint64(row.assertParam)), bytesField(row.declaration), bytesField(row.operand), bytesField(row.scope), bytesField(row.assertionNarrow), uintField(uint64(row.assertionCoordinate[0])), uintField(uint64(row.assertionCoordinate[1])), uintField(uint64(row.assertionCoordinate[2])), uintField(uint64(row.assertionCoordinate[3])), bytesField(row.typeFunctionVariadic), uintField(uint64(len(row.aliasParams))))
		for _, child := range row.aliasParams {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.interfaceExtends))))
		for _, child := range row.interfaceExtends {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.interfaceMemberTypes))))
		for _, child := range row.interfaceMemberTypes {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.typeFunctionTypeParams))))
		for _, child := range row.typeFunctionTypeParams {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.typeFunctionParams))))
		for _, child := range row.typeFunctionParams {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.typeFunctionReturns))))
		for _, child := range row.typeFunctionReturns {
			sink.add(bytesField(child))
		}
		sink.add(uintField(uint64(len(row.fieldKeys))))
		for index, key := range row.fieldKeys {
			sink.add(uintField(uint64(key)))
			text := ""
			if index < len(row.fieldTexts) {
				text = row.fieldTexts[index]
			}
			optional := false
			if index < len(row.fieldOptional) {
				optional = row.fieldOptional[index]
			}
			readonly := false
			if index < len(row.fieldReadonly) {
				readonly = row.fieldReadonly[index]
			}
			sink.add(field{bytes: []byte(text), kind: fieldBytes}, boolField(optional), boolField(readonly))
		}
		sink.add(uintField(uint64(len(row.keys))))
		for _, key := range row.keys {
			sink.add(uintField(uint64(key)))
		}
		for index := range row.keys {
			text := ""
			if index < len(row.texts) {
				text = row.texts[index]
			}
			sink.add(field{bytes: []byte(text), kind: fieldBytes})
			optional := false
			if index < len(row.optional) {
				optional = row.optional[index]
			}
			memberKind := uint8(0)
			if index < len(row.memberKinds) {
				memberKind = row.memberKinds[index]
			}
			sink.add(boolField(optional), uintField(uint64(memberKind)))
		}
		sink.add(uintField(uint64(len(row.segments))))
		for _, segment := range row.segments {
			sink.add(uintField(uint64(segment)))
		}
		sink.add(boolField(row.returnsKnown))
		sink.add(uintField(uint64(len(row.sourceKeys))))
		for _, key := range row.sourceKeys {
			sink.add(uintField(uint64(key)))
		}
		sink.add(uintField(uint64(len(row.canonicalKeys))))
		for _, key := range row.canonicalKeys {
			sink.add(uintField(uint64(key)))
		}
		sink.add(uintField(uint64(len(row.children))))
		for _, child := range row.children {
			sink.add(bytesField(child))
		}
	}
	sink.add(uintField(uint64(len(artifact.staticExpressions))))
	for _, row := range artifact.staticExpressions {
		sink.add(bytesField(row.id), bytesField(row.reference), bytesField(row.owner))
	}
	sink.add(uintField(uint64(len(artifact.staticInputs))))
	for _, row := range artifact.staticInputs {
		exact := row.literal
		sink.add(bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), uintField(uint64(row.operandKind)), bytesField(row.expression), bytesField(row.source), bytesField(row.target), bytesField(row.operand), bytesField(row.frontier), bytesField(row.operandReference), bytesField(row.operandSubject), bytesField(row.operandBody), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.cursor)))
	}
	sink.add(uintField(uint64(len(artifact.environment))))
	for _, edge := range artifact.environment {
		sink.add(
			bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), bytesField(edge.route),
			uintField(uint64(edge.arm)), bytesField(edge.guard), bytesField(edge.decision), bytesField(edge.condition), boolField(edge.guarded), boolField(edge.truth),
			bytesField(edge.component), bytesField(edge.mu), boolField(edge.hasMu),
			bytesField(edge.reset), boolField(edge.hasReset), uintField(uint64(len(edge.resets))),
		)
		for _, reset := range edge.resets {
			sink.add(bytesField(reset))
		}
	}
	sink.add(uintField(uint64(len(artifact.localTransfers))))
	for _, edge := range artifact.localTransfers {
		sink.add(bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), boolField(edge.full), uintField(uint64(len(edge.writes))))
		for _, write := range edge.writes {
			sink.add(keyField(write))
		}
	}
	sink.add(uintField(uint64(len(artifact.ruleOccurrences))))
	for _, row := range artifact.ruleOccurrences {
		sink.add(
			keyField(row.key), uintField(uint64(row.occurrence)), bytesField(row.point), bytesField(row.input),
			uintField(uint64(row.stage)), uintField(uint64(row.inputKind)), bytesField(row.route),
		)
	}
	sink.add(uintField(uint64(len(artifact.regions))))
	for _, region := range artifact.regions {
		sink.add(
			bytesField(region.id), bytesField(region.parent), boolField(region.cyclic),
			uintField(uint64(len(region.members))),
		)
		for _, member := range region.members {
			sink.add(bytesField(member))
		}
	}
	sink.add(uintField(uint64(len(artifact.events))))
	for _, event := range artifact.events {
		sink.add(uintField(uint64(event.kind)), bytesField(event.region), bytesField(event.point))
	}
	return sink.sum()
}
