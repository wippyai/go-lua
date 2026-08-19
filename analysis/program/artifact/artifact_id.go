package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/program"
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
	// Points and their decision plane are read out of the sealed cold
	// publication. The decision span preserves the emitted order, so the
	// preimage is the same sequence the identity has always committed to.
	pointCount, pointsPublished := coldCount(artifact, programschema.PointFamily())
	if !pointsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(pointCount)))
	for index := 0; index < pointCount; index++ {
		point, held := coldRow(artifact, programschema.PointFamily(), index)
		offset, decisions, spanOK := point.DecisionSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(point.ID()), boolField(point.Initial()), uintField(uint64(decisions)))
		for position := uint32(0); position < decisions; position++ {
			decision, decisionHeld := coldRow(artifact, programschema.PointDecisionFamily(), int(offset+position))
			if !decisionHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(decision.ID()))
		}
	}
	// Values and its member plane are read out of the sealed cold publication.
	// The member span preserves the emitted order, so the preimage is the same
	// sequence the identity has always committed to.
	valuesCount, valuesPublished := coldCount(artifact, programschema.ValuesFamily())
	if !valuesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(valuesCount)))
	for index := 0; index < valuesCount; index++ {
		row, held := coldRow(artifact, programschema.ValuesFamily(), index)
		offset, members, spanOK := row.MemberSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.BodyPathID()), uintField(uint64(members)))
		for position := uint32(0); position < members; position++ {
			member, memberHeld := coldRow(artifact, programschema.ValuesMemberFamily(), int(offset+position))
			if !memberHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(member.ID()))
		}
		tail, present := row.Tail()
		sink.add(boolField(present), uintField(uint64(tail.Kind())), bytesField(tail.ID()))
	}
	// Calls are read from their sealed cold families. Keep every scalar and
	// ordered child identity in the artifact seal so replay, mutation, and
	// mounted joins authenticate the direct plane exactly.
	callCount, callsPublished := coldCount(artifact, programschema.CallFamily())
	operandCount, operandsPublished := coldCount(artifact, programschema.CallOperandFamily())
	argumentCount, argumentsPublished := coldCount(artifact, programschema.CallArgumentFamily())
	typeArgumentCount, typeArgumentsPublished := coldCount(artifact, programschema.CallTypeArgumentFamily())
	if !callsPublished || !operandsPublished || !argumentsPublished || !typeArgumentsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(callRowsLawVersion), uintField(uint64(callCount)))
	for index := 0; index < callCount; index++ {
		row, held := coldRow(artifact, programschema.CallFamily(), index)
		operandStart, operandWidth, operandSpanOK := row.OperandSpan()
		argumentStart, argumentWidth, argumentSpanOK := row.ArgumentSpan()
		typeArgumentStart, typeArgumentWidth, typeArgumentSpanOK := row.TypeArgumentSpan()
		if !held || !row.Available() || !operandSpanOK || !argumentSpanOK || !typeArgumentSpanOK ||
			uint64(operandStart)+uint64(operandWidth) > uint64(operandCount) ||
			uint64(argumentStart)+uint64(argumentWidth) > uint64(argumentCount) ||
			uint64(typeArgumentStart)+uint64(typeArgumentWidth) > uint64(typeArgumentCount) {
			return identity.ContentID{}
		}
		receiver, hasReceiver := row.ReceiverID()
		tail, hasTail := row.TailID()
		target, _ := row.DirectTargetBody()
		sink.add(
			bytesField(row.ID()), bytesField(row.BodyID()), bytesField(row.SpanID()), bytesField(row.FormalID()),
			bytesField(row.ValuesID()), bytesField(row.ValuesRootID()), bytesField(row.TypeArgumentsID()), bytesField(row.CalleeID()), bytesField(row.ActualsID()),
			bytesField(target),
			uintField(uint64(row.Form())), boolField(hasReceiver), bytesField(receiver), boolField(hasTail), bytesField(tail),
			uintField(uint64(operandWidth)), uintField(uint64(argumentWidth)), uintField(uint64(typeArgumentWidth)),
		)
		for childIndex := uint32(0); childIndex < operandWidth; childIndex++ {
			operand, childHeld := coldRow(artifact, programschema.CallOperandFamily(), int(operandStart+childIndex))
			if !childHeld || !operand.Available() {
				return identity.ContentID{}
			}
			sink.add(bytesField(operand.ID()), bytesField(operand.CallID()), bytesField(operand.ValueID()), bytesField(operand.SpanID()), uintField(uint64(operand.Kind())))
		}
		for childIndex := uint32(0); childIndex < argumentWidth; childIndex++ {
			argument, childHeld := coldRow(artifact, programschema.CallArgumentFamily(), int(argumentStart+childIndex))
			if !childHeld || !argument.Available() {
				return identity.ContentID{}
			}
			sink.add(bytesField(argument.ID()), bytesField(argument.CallID()), bytesField(argument.ValuesID()), bytesField(argument.MemberID()), bytesField(argument.SpanID()), uintField(uint64(argument.Index())))
		}
		for childIndex := uint32(0); childIndex < typeArgumentWidth; childIndex++ {
			argument, childHeld := coldRow(artifact, programschema.CallTypeArgumentFamily(), int(typeArgumentStart+childIndex))
			if !childHeld || !argument.Available() {
				return identity.ContentID{}
			}
			sink.add(bytesField(argument.ID()), bytesField(argument.CallID()), bytesField(argument.TypesID()), bytesField(argument.ReferenceID()), uintField(uint64(argument.Index())))
		}
	}
	bodyCount, bodiesPublished := coldCount(artifact, programschema.BodyFamily())
	if !bodiesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(bodyCount)))
	for index := 0; index < bodyCount; index++ {
		body, held := coldRow(artifact, programschema.BodyFamily(), index)
		entryOffset, entryCount, entriesOK := body.EntrySpan()
		rootOffset, rootCount, rootsOK := body.RootSpan()
		outcomeOffset, outcomeCount, outcomesOK := body.OutcomeSpan()
		function, _ := body.FunctionContextID()
		formal, _ := body.CallFormalID()
		if !held || !entriesOK || !rootsOK || !outcomesOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(body.ID()), bytesField(body.ContextID()), bytesField(body.EntryID()), boolField(body.Callable()), bytesField(function), bytesField(formal), uintField(uint64(entryCount)))
		for position := uint32(0); position < entryCount; position++ {
			point, childHeld := coldRow(artifact, programschema.BodyEntryFamily(), int(entryOffset+position))
			if !childHeld || point.BodyID() != body.ID() {
				return identity.ContentID{}
			}
			sink.add(bytesField(point.PointID()))
		}
		sink.add(uintField(uint64(rootCount)))
		for position := uint32(0); position < rootCount; position++ {
			root, childHeld := coldRow(artifact, programschema.BodyRootFamily(), int(rootOffset+position))
			if !childHeld || root.BodyID() != body.ID() {
				return identity.ContentID{}
			}
			sink.add(bytesField(root.ID()), uintField(uint64(root.Family())))
		}
		sink.add(uintField(uint64(outcomeOffset)), uintField(uint64(outcomeOffset+outcomeCount)))
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
	targets, published := programschema.CallTargetFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !published {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(targets)))
	for index := 0; index < targets; index++ {
		target, held := programschema.CallTargetFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(bytesField(target.Allocation), bytesField(target.Body), bytesField(target.Context), bytesField(target.Function), bytesField(target.Formal))
	}
	outcomeCount, outcomesPublished := coldCount(artifact, programschema.OutcomeFamily())
	if !outcomesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(outcomeCount)))
	for index := 0; index < outcomeCount; index++ {
		outcome, held := coldRow(artifact, programschema.OutcomeFamily(), index)
		returnOffset, returnCount, returnsOK := outcome.ReturnValueSpan()
		pointOffset, pointCount, pointsOK := outcome.PointSpan()
		target, hasTarget := outcome.TargetID()
		propagation, hasPropagation := outcome.PropagationID()
		if !held || !returnsOK || !pointsOK {
			return identity.ContentID{}
		}
		sink.add(
			bytesField(outcome.ID()), bytesField(outcome.BodyID()), uintField(uint64(outcome.Kind())),
			boolField(hasTarget), bytesField(target),
			boolField(hasPropagation), bytesField(propagation),
			uintField(uint64(returnOffset)), uintField(uint64(returnOffset+returnCount)), uintField(uint64(pointCount)),
		)
		for position := uint32(0); position < pointCount; position++ {
			point, childHeld := coldRow(artifact, programschema.OutcomePointFamily(), int(pointOffset+position))
			if !childHeld || point.OutcomeID() != outcome.ID() {
				return identity.ContentID{}
			}
			sink.add(bytesField(point.PointID()))
		}
	}
	returnValueCount, returnsPublished := coldCount(artifact, programschema.OutcomeReturnValueFamily())
	if !returnsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(returnValueCount)))
	for index := 0; index < returnValueCount; index++ {
		value, held := coldRow(artifact, programschema.OutcomeReturnValueFamily(), index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(bytesField(value.ValuesID()))
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
	exactCount, exactPublished := programschema.ExactScalarSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !exactPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(exactCount)))
	for index := 0; index < exactCount; index++ {
		row, rowOK := programschema.ExactScalarSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		literal, literalOK := row.Literal()
		if !rowOK || !literalOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.SubjectID()), bytesField(row.BodyPathID()),
			uintField(uint64(row.Role())), uintField(uint64(literal.Kind)), uintField(uint64(literal.Integer)), uintField(literal.FloatBits))
	}
	arithmeticCount, arithmeticPublished := programschema.ArithmeticSummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !arithmeticPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(arithmeticCount)))
	for index := 0; index < arithmeticCount; index++ {
		row, rowOK := programschema.ArithmeticSummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		left, right, result, representationsOK := row.Representations()
		if !rowOK || !representationsOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.BodyPathID()), uintField(uint64(row.Operator())),
			uintField(uint64(left)), uintField(uint64(right)), uintField(uint64(result)), uintField(uint64(row.DivisorProperty())))
	}
	unaryCount, unaryPublished := programschema.UnarySummaryFamily().Count(&artifact.frozen, artifact.coldCatalog)
	if !unaryPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(unaryCount)))
	for index := 0; index < unaryCount; index++ {
		row, rowOK := programschema.UnarySummaryFamily().At(&artifact.frozen, artifact.coldCatalog, index)
		operand, result, representationsOK := row.Representations()
		if !rowOK || !representationsOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.OccurrenceID()), bytesField(row.BodyPathID()), bytesField(row.OutputPointID()), uintField(uint64(row.Operator())),
			uintField(uint64(operand)), uintField(uint64(result)))
	}
	allocationCount, allocationsPublished := coldCount(artifact, programschema.HeapAllocationFamily())
	if !allocationsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(allocationCount)))
	for index := 0; index < allocationCount; index++ {
		allocation, held := coldRow(artifact, programschema.HeapAllocationFamily(), index)
		offset, fields, spanOK := allocation.FieldSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(allocation.ID()), uintField(uint64(allocation.Role())), uintField(uint64(allocation.Form())), bytesField(allocation.RootSpan()), uintField(uint64(fields)))
		for position := uint32(0); position < fields; position++ {
			field, fieldHeld := coldRow(artifact, programschema.HeapFieldFamily(), int(offset+position))
			valuesSpan, width, finalOpen, valuesOK := field.Values()
			normalized, normalizedOK := field.NormalizedKey()
			if !fieldHeld || !valuesOK {
				return identity.ContentID{}
			}
			sink.add(bytesField(field.ID()), uintField(uint64(field.Kind())), bytesField(field.FieldSpan()), bytesField(field.SelectorSpan()), bytesField(valuesSpan), bytesField(field.ValuesID()), uintField(uint64(width)), boolField(finalOpen), boolField(field.SharesFirstValueCell()), uintField(normalized), boolField(normalizedOK))
		}
	}
	indexCount, indexesPublished := coldCount(artifact, programschema.HeapIndexFamily())
	if !indexesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(indexCount)))
	for index := 0; index < indexCount; index++ {
		access, held := coldRow(artifact, programschema.HeapIndexFamily(), index)
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
	// The authored type-value plane is read out of the sealed cold publication
	// in its emitted order, which is the order the identity has always
	// committed to.
	typeValueCount, typeValuesPublished := coldCount(artifact, programschema.StaticTypeValueFamily())
	if !typeValuesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(typeValueCount)))
	for index := 0; index < typeValueCount; index++ {
		row, held := coldRow(artifact, programschema.StaticTypeValueFamily(), index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.BodyPathID()), bytesField(row.ReferenceID()), bytesField(row.RootID()), field{bytes: []byte(row.Name()), kind: fieldBytes})
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
		sink.add(uintField(uint64(len(row.fieldReadonly))))
		for _, readonly := range row.fieldReadonly {
			sink.add(boolField(readonly))
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
	expressionCount, expressionsPublished := coldCount(artifact, programschema.StaticExpressionFamily())
	if !expressionsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(expressionCount)))
	for index := 0; index < expressionCount; index++ {
		row, held := coldRow(artifact, programschema.StaticExpressionFamily(), index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.ID()), bytesField(row.ReferenceID()), bytesField(row.Owner()))
	}
	sink.add(uintField(uint64(len(artifact.staticInputs))))
	for _, row := range artifact.staticInputs {
		exact := row.literal
		sink.add(bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), uintField(uint64(row.operandKind)), bytesField(row.expression), bytesField(row.source), bytesField(row.target), bytesField(row.operand), bytesField(row.frontier), bytesField(row.operandReference), bytesField(row.operandSubject), bytesField(row.operandBody), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.cursor)))
	}
	// The environment plane and its reset witnesses are read out of the sealed
	// cold publication. The witness span preserves the emitted order, so the
	// preimage is the same sequence the identity has always committed to.
	edgeCount, edgesPublished := coldCount(artifact, programschema.EnvironmentEdgeFamily())
	if !edgesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(edgeCount)))
	for index := 0; index < edgeCount; index++ {
		edge, held := coldRow(artifact, programschema.EnvironmentEdgeFamily(), index)
		offset, resets, spanOK := edge.ResetSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		guard, _ := edge.GuardID()
		decision, _ := edge.DecisionID()
		condition, _ := edge.ConditionValueSpanID()
		truth, guarded := edge.Truth()
		mu, hasMu := edge.MuPathID()
		reset, hasReset := edge.ResetDigest()
		sink.add(
			bytesField(edge.ID()), bytesField(edge.From()), bytesField(edge.To()), bytesField(edge.RouteID()),
			uintField(uint64(edge.Arm())), bytesField(guard), bytesField(decision), bytesField(condition), boolField(guarded), boolField(truth),
			bytesField(edge.ComponentID()), bytesField(mu), boolField(hasMu),
			bytesField(reset), boolField(hasReset), uintField(uint64(resets)),
		)
		for position := uint32(0); position < resets; position++ {
			witness, witnessHeld := coldRow(artifact, programschema.EnvironmentResetFamily(), int(offset+position))
			if !witnessHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(witness.ID()))
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
	// The region plane, its member plane and the event bracket sequence are
	// read out of the sealed cold publication. The member span preserves the
	// emitted order, so the preimage is the same sequence the identity has
	// always committed to.
	regionCount, regionsPublished := coldCount(artifact, programschema.RegionFamily())
	if !regionsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(regionCount)))
	for index := 0; index < regionCount; index++ {
		region, held := coldRow(artifact, programschema.RegionFamily(), index)
		offset, members, spanOK := region.MemberSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(
			bytesField(region.ID()), bytesField(region.ParentID()), boolField(region.Cyclic()),
			uintField(uint64(members)),
		)
		for position := uint32(0); position < members; position++ {
			member, memberHeld := coldRow(artifact, programschema.RegionMemberFamily(), int(offset+position))
			if !memberHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(member.ID()))
		}
	}
	eventCount, eventsPublished := coldCount(artifact, programschema.WTOEventFamily())
	if !eventsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(eventCount)))
	for index := 0; index < eventCount; index++ {
		event, held := coldRow(artifact, programschema.WTOEventFamily(), index)
		if !held {
			return identity.ContentID{}
		}
		sink.add(uintField(uint64(event.Kind())), bytesField(event.RegionID()), bytesField(event.PointID()))
	}
	return sink.sum()
}
