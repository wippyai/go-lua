package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

type staticNodeDigestChildRow interface {
	programschema.Row
	ParentID() identity.ContentID
	ChildID() identity.ContentID
	Position() uint32
}

type staticNodeDigestMetadata struct {
	key      keyspace.Key
	text     string
	optional bool
	kind     uint8
}

// addStaticNodeChildFamily replays one typed dense child span into the
// historical identity stream. The family remains generic only at this
// transport boundary; each caller supplies a distinct schema family.
func addStaticNodeChildFamily[V staticNodeDigestChildRow](sink *digestSink, artifact *Artifact, parent identity.ContentID, offset, count uint32, family programschema.Family[V]) bool {
	sink.add(uintField(uint64(count)))
	for position := uint32(0); position < count; position++ {
		child, ok := family.At(&artifact.frozen, artifact.coldCatalog, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position {
			return false
		}
		sink.add(bytesField(child.ChildID()))
	}
	return true
}

func addStaticNodeReadonlyFamily[V staticNodeDigestChildRow](sink *digestSink, artifact *Artifact, parent identity.ContentID, offset, count uint32, family programschema.Family[V], readonly func(V) bool) bool {
	sink.add(uintField(uint64(count)))
	for position := uint32(0); position < count; position++ {
		child, ok := family.At(&artifact.frozen, artifact.coldCatalog, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position {
			return false
		}
		sink.add(boolField(readonly(child)))
	}
	return true
}

func staticNodeMetadataSpan[V staticNodeDigestChildRow](artifact *Artifact, parent identity.ContentID, offset, count uint32, family programschema.Family[V], read func(V) staticNodeDigestMetadata) ([]staticNodeDigestMetadata, bool) {
	rows := make([]staticNodeDigestMetadata, 0, count)
	for position := uint32(0); position < count; position++ {
		child, ok := family.At(&artifact.frozen, artifact.coldCatalog, int(offset+position))
		if !ok || !child.Available() || child.ParentID() != parent || child.Position() != position {
			return nil, false
		}
		metadata := read(child)
		if metadata.key == 0 {
			return nil, false
		}
		rows = append(rows, metadata)
	}
	return rows, true
}

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
	callResultCount, callResultsPublished := coldCount(artifact, programschema.CallResultFamily())
	if !callResultsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(callResultRowsLawVersion), uintField(uint64(callResultCount)))
	for index := 0; index < callResultCount; index++ {
		row, held := coldRow(artifact, programschema.CallResultFamily(), index)
		value, hasValue := row.ValueID()
		tail, hasTail := row.ValuesTailID()
		position, hasPosition := row.Position()
		multiplicity := row.Multiplicity()
		count, hasCount := row.ResultCount()
		open, openOK := row.ResultsOpen()
		if !held || !row.Available() || hasValue == hasTail || hasPosition != (row.Form() == programschema.CallResultValue) || !multiplicity.Valid() || !openOK || open != (multiplicity == programschema.CallResultMultiplicityOpen) || hasCount != (multiplicity == programschema.CallResultMultiplicityExact) {
			return identity.ContentID{}
		}
		sink.add(bytesField(row.CallID()), bytesField(row.ValuesID()), uintField(uint64(row.Form())), uintField(uint64(multiplicity)), uintField(uint64(count)), boolField(open), boolField(hasValue), bytesField(value), boolField(hasTail), bytesField(tail), boolField(hasPosition), uintField(uint64(position)))
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
	boundaryCount, boundariesPublished := coldCount(artifact, programschema.FunctionBoundaryFamily())
	formalCount, formalsPublished := coldCount(artifact, programschema.FunctionFormalFamily())
	varargCount, varargsPublished := coldCount(artifact, programschema.FunctionVarargFamily())
	captureCount, capturesPublished := coldCount(artifact, programschema.FunctionCaptureFamily())
	if !boundariesPublished || !formalsPublished || !varargsPublished || !capturesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(functionBoundaryLawVersion), uintField(uint64(boundaryCount)))
	for index := 0; index < boundaryCount; index++ {
		boundary, boundaryHeld := coldRow(artifact, programschema.FunctionBoundaryFamily(), index)
		formalOffset, formalWidth, formalSpanOK := boundary.FormalSpan()
		varargOffset, varargWidth, varargSpanOK := boundary.VarargSpan()
		captureOffset, captureWidth, captureSpanOK := boundary.CaptureSpan()
		if !boundaryHeld || !boundary.Available() || !formalSpanOK || !varargSpanOK || !captureSpanOK ||
			uint64(formalOffset)+uint64(formalWidth) > uint64(formalCount) ||
			uint64(varargOffset)+uint64(varargWidth) > uint64(varargCount) ||
			uint64(captureOffset)+uint64(captureWidth) > uint64(captureCount) {
			return identity.ContentID{}
		}
		sink.add(
			bytesField(boundary.ID()), bytesField(boundary.BodyID()), bytesField(boundary.BodyContextID()), bytesField(boundary.EntryID()), bytesField(boundary.CallFormalID()),
			uintField(uint64(formalWidth)),
		)
		for position := uint32(0); position < formalWidth; position++ {
			port, portHeld := coldRow(artifact, programschema.FunctionFormalFamily(), int(formalOffset+position))
			if !portHeld || !port.Available() {
				return identity.ContentID{}
			}
			declared, _ := port.DeclaredStaticTypeID()
			formalPosition, positionOK := port.Position()
			if !positionOK {
				return identity.ContentID{}
			}
			sink.add(bytesField(port.ID()), bytesField(port.CellID()), bytesField(port.StorageCellID()), bytesField(declared), uintField(uint64(formalPosition)))
		}
		varargID, varargCell := identity.ContentID{}, identity.ContentID{}
		if varargWidth == 1 {
			vararg, varargHeld := coldRow(artifact, programschema.FunctionVarargFamily(), int(varargOffset))
			if !varargHeld || !vararg.Available() {
				return identity.ContentID{}
			}
			varargID, varargCell = vararg.ID(), vararg.CellID()
		}
		sink.add(boolField(varargWidth == 1), bytesField(varargID), bytesField(varargCell), uintField(uint64(captureWidth)))
		for position := uint32(0); position < captureWidth; position++ {
			capture, captureHeld := coldRow(artifact, programschema.FunctionCaptureFamily(), int(captureOffset+position))
			if !captureHeld || !capture.Available() {
				return identity.ContentID{}
			}
			capturePosition, positionOK := capture.Position()
			if !positionOK {
				return identity.ContentID{}
			}
			sink.add(
				bytesField(capture.ID()), bytesField(capture.InnerCellID()), bytesField(capture.OuterCellID()), bytesField(capture.InnerBodyID()), bytesField(capture.OuterBodyID()), uintField(uint64(capturePosition)),
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
	occurrenceCount, occurrencesPublished := coldCount(artifact, programschema.OccurrenceFamily())
	pointCount, occurrencePointsPublished := coldCount(artifact, programschema.OccurrencePointFamily())
	inputCount, occurrenceInputsPublished := coldCount(artifact, programschema.OccurrenceInputFamily())
	if !occurrencesPublished || !occurrencePointsPublished || !occurrenceInputsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(occurrenceCount)))
	for index := 0; index < occurrenceCount; index++ {
		row, held := coldRow(artifact, programschema.OccurrenceFamily(), index)
		pointOffset, points, pointSpanOK := row.PointSpan()
		inputOffset, inputs, inputSpanOK := row.InputSpan()
		literalFamily, literal, literalOK := row.Literal()
		if !held || !pointSpanOK || !inputSpanOK || uint64(pointOffset)+uint64(points) > uint64(pointCount) || uint64(inputOffset)+uint64(inputs) > uint64(inputCount) {
			return identity.ContentID{}
		}
		body, _ := row.BodyID()
		sink.add(uintField(uint64(row.Kind())), bytesField(row.ID()), bytesField(body), uintField(row.Code()), uintField(uint64(points)))
		for position := uint32(0); position < points; position++ {
			point, pointHeld := coldRow(artifact, programschema.OccurrencePointFamily(), int(pointOffset+position))
			if !pointHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(point.PointID()))
		}
		sink.add(uintField(uint64(inputs)))
		for position := uint32(0); position < inputs; position++ {
			input, inputHeld := coldRow(artifact, programschema.OccurrenceInputFamily(), int(inputOffset+position))
			if !inputHeld {
				return identity.ContentID{}
			}
			sink.add(bytesField(input.InputID()))
		}
		sink.add(uintField(uint64(literalFamily)), boolField(literalOK), uintField(uint64(literal.Kind)), boolField(literal.Bool), uintField(uint64(literal.Integer)), uintField(literal.FloatBits), field{bytes: []byte(literal.String), kind: fieldBytes})
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
	diagnosticCount, diagnosticsPublished := coldCount(artifact, programschema.DiagnosticObservationFamily())
	evidenceCount, evidencePublished := coldCount(artifact, programschema.DiagnosticEvidenceFamily())
	pathCount, pathsPublished := coldCount(artifact, programschema.DiagnosticPathFamily())
	if !diagnosticsPublished || !evidencePublished || !pathsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(diagnosticLawVersion), uintField(uint64(diagnosticCount)))
	for index := 0; index < diagnosticCount; index++ {
		row, held := coldRow(artifact, programschema.DiagnosticObservationFamily(), index)
		location, locationOK := row.Location()
		evidenceOffset, evidenceWidth, evidenceSpanOK := row.EvidenceSpan()
		pathOffset, pathWidth, pathSpanOK := row.PathSpan()
		position, positionOK := row.Position()
		if !held || !row.Available() || !locationOK || !evidenceSpanOK || !pathSpanOK ||
			!positionOK && row.Kind() == structure.DiagnosticObservationTypeConformance ||
			uint64(evidenceOffset)+uint64(evidenceWidth) > uint64(evidenceCount) || uint64(pathOffset)+uint64(pathWidth) > uint64(pathCount) {
			return identity.ContentID{}
		}
		sink.add(
			bytesField(row.ID()), uintField(uint64(row.Kind())),
			field{bytes: []byte(location.File), kind: fieldBytes}, uintField(uint64(location.StartLine)),
			uintField(uint64(location.StartCol)), uintField(uint64(location.EndLine)), uintField(uint64(location.EndCol)),
		)
		switch row.Kind() {
		case structure.DiagnosticObservationBranchCondition:
			sink.add(bytesField(row.DecisionPathID()), bytesField(row.ValueSpanID()), uintField(uint64(evidenceWidth)))
			for position := uint32(0); position < evidenceWidth; position++ {
				point, pointHeld := coldRow(artifact, programschema.DiagnosticEvidenceFamily(), int(evidenceOffset+position))
				if !pointHeld || !point.Available() {
					return identity.ContentID{}
				}
				sink.add(bytesField(point.PointID()))
			}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			sink.add(bytesField(row.StaticReferenceID()), bytesField(row.RootID()), uintField(uint64(pathWidth)))
			for position := uint32(0); position < pathWidth; position++ {
				component, componentHeld := coldRow(artifact, programschema.DiagnosticPathFamily(), int(pathOffset+position))
				if !componentHeld || !component.Available() {
					return identity.ContentID{}
				}
				sink.add(field{bytes: []byte(component.Component()), kind: fieldBytes})
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			sink.add(bytesField(row.ReadID()), bytesField(row.CellID()), field{bytes: []byte(row.Name()), kind: fieldBytes})
		case structure.DiagnosticObservationTypeConformance:
			sink.add(
				uintField(uint64(row.Site())), bytesField(row.OwnerID()), bytesField(row.MeasuredValueID()),
				bytesField(row.DeclaredStaticTypeID()), bytesField(row.SpanID()), uintField(uint64(position)),
				uintField(uint64(evidenceWidth)),
			)
			for position := uint32(0); position < evidenceWidth; position++ {
				point, pointHeld := coldRow(artifact, programschema.DiagnosticEvidenceFamily(), int(evidenceOffset+position))
				if !pointHeld || !point.Available() {
					return identity.ContentID{}
				}
				sink.add(bytesField(point.PointID()))
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
	typeNodeCount, typeNodesPublished := coldCount(artifact, programschema.StaticTypeNodeFamily())
	if !typeNodesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(typeNodeCount)))
	program := programschema.Program{Frozen: artifact.frozen, ProgramID: artifact.key.ProgramID(), SchemaID: artifact.key.SchemaDigest()}
	// Replay the historical StaticTypeNode preimage from the canonical parent
	// and typed child/metadata families. The order and fields below are kept
	// byte-for-byte identical to the former artifact-local row walk; spans are
	// storage only and never enter this digest.
	for index := 0; index < typeNodeCount; index++ {
		row, held := coldRow(artifact, programschema.StaticTypeNodeFamily(), index)
		if !held || !row.Available() {
			return identity.ContentID{}
		}
		exact := row.Exact()
		sink.add(bytesField(row.ID()), bytesField(row.Owner()), uintField(uint64(row.Kind())), field{bytes: []byte(row.Name()), kind: fieldBytes}, uintField(uint64(row.Key())), uintField(uint64(row.LiteralKind())), uintField(row.Bits()), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, boolField(row.Flag()), uintField(uint64(row.Resolution())), uintField(uint64(row.AssertionParam())))
		declaration, _ := row.DeclarationOwner()
		operand, _ := row.OperandID()
		scope, _ := row.ScopeID()
		narrow, _ := row.AssertionNarrowID()
		variadic, _ := row.TypeFunctionVariadic()
		c0, c1, c2, c3 := row.AssertionCoordinate()
		sink.add(bytesField(declaration), bytesField(operand), bytesField(scope), bytesField(narrow), uintField(uint64(c0)), uintField(uint64(c1)), uintField(uint64(c2)), uintField(uint64(c3)), bytesField(variadic))
		aliasOffset, aliasCount, aliasOK := row.AliasParameterSpan()
		extendOffset, extendCount, extendOK := row.InterfaceExtendSpan()
		memberOffset, memberCount, memberOK := row.InterfaceMemberSpan()
		typeParamOffset, typeParamCount, typeParamOK := row.TypeFunctionTypeParameterSpan()
		parameterOffset, parameterCount, parameterOK := row.TypeFunctionParameterSpan()
		returnOffset, returnCount, returnOK := row.TypeFunctionReturnSpan()
		recordOffset, recordCount, recordOK := row.RecordFieldSpan()
		if !aliasOK || !extendOK || !memberOK || !typeParamOK || !parameterOK || !returnOK || !recordOK {
			return identity.ContentID{}
		}
		if !addStaticNodeChildFamily(&sink, artifact, row.ID(), aliasOffset, aliasCount, programschema.StaticTypeNodeAliasParameterFamily()) ||
			!addStaticNodeChildFamily(&sink, artifact, row.ID(), extendOffset, extendCount, programschema.StaticTypeNodeInterfaceExtendFamily()) ||
			!addStaticNodeChildFamily(&sink, artifact, row.ID(), memberOffset, memberCount, programschema.StaticTypeNodeInterfaceMemberFamily()) ||
			!addStaticNodeChildFamily(&sink, artifact, row.ID(), typeParamOffset, typeParamCount, programschema.StaticTypeNodeTypeFunctionTypeParameterFamily()) ||
			!addStaticNodeChildFamily(&sink, artifact, row.ID(), parameterOffset, parameterCount, programschema.StaticTypeNodeTypeFunctionParameterFamily()) ||
			!addStaticNodeChildFamily(&sink, artifact, row.ID(), returnOffset, returnCount, programschema.StaticTypeNodeTypeFunctionReturnFamily()) {
			return identity.ContentID{}
		}
		fieldReadonlyCount := uint32(0)
		if row.Kind() == programschema.StaticNodeRecord {
			fieldReadonlyCount = recordCount
		} else if row.Kind() == programschema.StaticNodeInterface {
			fieldReadonlyCount = memberCount
		}
		if row.Kind() == programschema.StaticNodeRecord {
			if !addStaticNodeReadonlyFamily(&sink, artifact, row.ID(), recordOffset, fieldReadonlyCount, programschema.StaticTypeNodeRecordFieldFamily(), func(field programschema.StaticTypeNodeRecordField) bool { return field.Readonly() }) {
				return identity.ContentID{}
			}
		} else if row.Kind() == programschema.StaticNodeInterface {
			if !addStaticNodeReadonlyFamily(&sink, artifact, row.ID(), memberOffset, fieldReadonlyCount, programschema.StaticTypeNodeInterfaceMemberFamily(), func(field programschema.StaticTypeNodeInterfaceMember) bool { return field.Readonly() }) {
				return identity.ContentID{}
			}
		} else {
			sink.add(uintField(uint64(fieldReadonlyCount)))
		}
		var metadata []staticNodeDigestMetadata
		var metadataOK bool
		switch row.Kind() {
		case programschema.StaticNodeRecord:
			metadata, metadataOK = staticNodeMetadataSpan(artifact, row.ID(), recordOffset, recordCount, programschema.StaticTypeNodeRecordFieldFamily(), func(field programschema.StaticTypeNodeRecordField) staticNodeDigestMetadata {
				return staticNodeDigestMetadata{key: field.Key(), text: field.Text(), optional: field.Optional()}
			})
		case programschema.StaticNodeInterface:
			metadata, metadataOK = staticNodeMetadataSpan(artifact, row.ID(), memberOffset, memberCount, programschema.StaticTypeNodeInterfaceMemberFamily(), func(member programschema.StaticTypeNodeInterfaceMember) staticNodeDigestMetadata {
				return staticNodeDigestMetadata{key: member.Key(), text: member.Text(), optional: member.Optional(), kind: member.KindCode()}
			})
		case programschema.StaticNodeTypeFunction:
			metadata, metadataOK = staticNodeMetadataSpan(artifact, row.ID(), parameterOffset, parameterCount, programschema.StaticTypeNodeTypeFunctionParameterFamily(), func(parameter programschema.StaticTypeNodeTypeFunctionParameter) staticNodeDigestMetadata {
				return staticNodeDigestMetadata{key: parameter.Key(), text: parameter.Text()}
			})
		default:
			metadataOK = true
		}
		if !metadataOK {
			return identity.ContentID{}
		}
		sink.add(uintField(uint64(len(metadata))))
		for _, item := range metadata {
			sink.add(uintField(uint64(item.key)))
		}
		for _, item := range metadata {
			sink.add(field{bytes: []byte(item.text), kind: fieldBytes}, boolField(item.optional), uintField(uint64(item.kind)))
		}
		segmentCount := row.SegmentCount()
		sink.add(uintField(uint64(segmentCount)))
		for n := 0; n < segmentCount; n++ {
			segment, segmentOK := row.SegmentAt(n)
			if !segmentOK {
				return identity.ContentID{}
			}
			sink.add(uintField(uint64(segment)))
		}
		sink.add(boolField(row.ReturnsKnown()))
		sourceOffset, sourceCount, sourceOK := row.ReferenceSourceKeySpan()
		canonicalOffset, canonicalCount, canonicalOK := row.ReferenceCanonicalKeySpan()
		if !sourceOK || !canonicalOK {
			return identity.ContentID{}
		}
		sink.add(uintField(uint64(sourceCount)))
		for n := uint32(0); n < sourceCount; n++ {
			key, keyOK := programschema.StaticTypeNodeReferenceSourceKeyFamily().At(&artifact.frozen, artifact.coldCatalog, int(sourceOffset+n))
			if !keyOK || key.ParentID() != row.ID() {
				return identity.ContentID{}
			}
			sink.add(uintField(uint64(key.Key())))
		}
		sink.add(uintField(uint64(canonicalCount)))
		for n := uint32(0); n < canonicalCount; n++ {
			key, keyOK := programschema.StaticTypeNodeReferenceCanonicalKeyFamily().At(&artifact.frozen, artifact.coldCatalog, int(canonicalOffset+n))
			if !keyOK || key.ParentID() != row.ID() {
				return identity.ContentID{}
			}
			sink.add(uintField(uint64(key.Key())))
		}
		childRows, childOK := program.StaticTypeNodeChildren(index, row, false)
		if !childOK {
			return identity.ContentID{}
		}
		sink.add(uintField(uint64(len(childRows))))
		for _, child := range childRows {
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
	inputCount, inputsPublished := coldCount(artifact, programschema.StaticInputFamily())
	if !inputsPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(inputCount)))
	for index := 0; index < inputCount; index++ {
		row, held := coldRow(artifact, programschema.StaticInputFamily(), index)
		if !held || !row.Available() {
			return identity.ContentID{}
		}
		exact := row.OperandLiteral()
		sink.add(bytesField(row.ID()), bytesField(row.Owner()), uintField(uint64(row.Kind())), uintField(uint64(row.OperandKind())), bytesField(row.ExpressionID()), bytesField(row.SourceID()), bytesField(row.TargetID()), bytesField(row.OperandID()), bytesField(row.FrontierID()), bytesField(row.OperandReferenceID()), bytesField(row.OperandSubjectID()), bytesField(row.OperandBodyPathID()), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.Cursor())))
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
	transferCount, transfersPublished := coldCount(artifact, programschema.LocalTransferFamily())
	if !transfersPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(transferCount)))
	for index := 0; index < transferCount; index++ {
		edge, held := coldRow(artifact, programschema.LocalTransferFamily(), index)
		offset, writeCount, spanOK := edge.WriteSpan()
		if !held || !spanOK {
			return identity.ContentID{}
		}
		sink.add(bytesField(edge.ID()), bytesField(edge.From()), bytesField(edge.To()), boolField(edge.Full()), uintField(uint64(writeCount)))
		for position := uint32(0); position < writeCount; position++ {
			write, writeOK := coldRow(artifact, programschema.LocalTransferWriteFamily(), int(offset+position))
			if !writeOK {
				return identity.ContentID{}
			}
			key, keyOK := write.Key()
			if !keyOK {
				return identity.ContentID{}
			}
			sink.add(keyField(key))
		}
	}
	ruleCount, rulesPublished := coldCount(artifact, programschema.RuleOccurrenceFamily())
	if !rulesPublished {
		return identity.ContentID{}
	}
	sink.add(uintField(uint64(ruleCount)))
	for index := 0; index < ruleCount; index++ {
		row, held := coldRow(artifact, programschema.RuleOccurrenceFamily(), index)
		occurrence, occurrenceOK := row.Occurrence()
		input, inputOK := row.InputPoint()
		route, routeOK := row.PredecessorRouteID()
		if !held || !occurrenceOK || !inputOK && row.InputKind() != programschema.RuleInputNone || !routeOK && row.InputKind() == programschema.RuleInputPredecessor {
			return identity.ContentID{}
		}
		key, writes := row.Key(), row.Writes()
		if !key.Available() || !writes.Available() {
			return identity.ContentID{}
		}
		sink.add(keyField(key), keyField(writes), uintField(uint64(occurrence)), bytesField(row.PointID()), bytesField(input), uintField(uint64(row.Stage())), uintField(uint64(row.InputKind())), bytesField(route))
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
