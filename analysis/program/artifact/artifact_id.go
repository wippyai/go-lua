package artifact

import (
	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/structure"
)

func artifactID(artifact *Artifact) identity.ContentID {
	fields := append([]field{bytesField(artifact.key.ID())}, artifact.key.identityFields()...)
	fields = append(fields, uintField(uint64(artifact.counts.Count())))
	for index := 0; index < artifact.counts.Count(); index++ {
		row, ok := artifact.counts.At(index)
		if !ok {
			return identity.ContentID{}
		}
		fields = append(fields, bytesField(identity.ContentID(row.ID())), uintField(row.Count()))
	}
	fields = append(fields, uintField(pointGeometryLawVersion))
	fields = append(fields, uintField(pointAttachmentLawVersion))
	fields = append(fields, uintField(uint64(len(artifact.points))))
	for _, point := range artifact.points {
		fields = append(fields, bytesField(point.id), boolField(point.initial), uintField(uint64(len(point.decisions))))
		for _, decision := range point.decisions {
			fields = append(fields, bytesField(decision))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.pointAttachments))))
	for _, row := range artifact.pointAttachments {
		fields = append(fields, bytesField(row.site), bytesField(row.point))
	}
	fields = append(fields, uintField(uint64(len(artifact.values))))
	for _, row := range artifact.values {
		fields = append(fields, bytesField(row.id), bytesField(row.body), uintField(uint64(len(row.members))))
		for _, member := range row.members {
			fields = append(fields, bytesField(member.id))
		}
		fields = append(fields, boolField(row.tail.present), uintField(uint64(row.tail.kind)), bytesField(row.tail.id))
	}
	// Calls are an Artifact-owned source column. Keep
	// every scalar and ordered child identity in the artifact seal so replay,
	// mutation, and mounted joins authenticate the direct plane exactly.
	fields = append(fields, uintField(callRowsLawVersion), uintField(uint64(len(artifact.calls))))
	for _, row := range artifact.calls {
		fields = append(fields,
			bytesField(row.id), bytesField(row.body), bytesField(row.span), bytesField(row.formal),
			bytesField(row.values), bytesField(row.valuesRoot), bytesField(row.types), bytesField(row.callee), bytesField(row.actuals),
			uintField(uint64(row.form)), boolField(row.hasReceiver), bytesField(row.receiver), boolField(row.hasTail), bytesField(row.tail),
			uintField(uint64(row.OperandCount())), uintField(uint64(row.ArgumentCount())), uintField(uint64(row.TypeArgumentCount())),
		)
		for index := int(row.operandStart); index < int(row.operandEnd); index++ {
			operand := artifact.callOperands[index]
			fields = append(fields, bytesField(operand.id), bytesField(operand.call), bytesField(operand.value), bytesField(operand.span), uintField(uint64(operand.kind)))
		}
		for index := int(row.argumentStart); index < int(row.argumentEnd); index++ {
			argument := artifact.callArguments[index]
			fields = append(fields, bytesField(argument.id), bytesField(argument.call), bytesField(argument.values), bytesField(argument.member), bytesField(argument.span), uintField(uint64(argument.position)))
		}
		for index := int(row.typeArgumentStart); index < int(row.typeArgumentEnd); index++ {
			argument := artifact.callTypeArguments[index]
			fields = append(fields, bytesField(argument.id), bytesField(argument.call), bytesField(argument.types), bytesField(argument.reference), uintField(uint64(argument.position)))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.bodies))))
	for _, body := range artifact.bodies {
		fields = append(fields, bytesField(body.id), bytesField(body.context), bytesField(body.entry), boolField(body.callable), bytesField(body.function), bytesField(body.formal), uintField(uint64(len(body.entryPoints))))
		for _, point := range body.entryPoints {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(body.roots))))
		for _, root := range body.roots {
			fields = append(fields, bytesField(root.id), uintField(uint64(root.family)))
		}
		fields = append(fields, uintField(uint64(body.outcomeStart)), uintField(uint64(body.outcomeEnd)))
	}
	fields = append(fields, uintField(functionBoundaryLawVersion), uintField(uint64(len(artifact.functionBoundaries))))
	for _, boundary := range artifact.functionBoundaries {
		fields = append(fields,
			bytesField(boundary.id), bytesField(boundary.body), bytesField(boundary.bodyContext), bytesField(boundary.entry), bytesField(boundary.callFormal),
			uintField(uint64(len(boundary.formals))),
		)
		for _, port := range boundary.formals {
			fields = append(fields, bytesField(port.id), bytesField(port.cell), bytesField(port.storage), bytesField(port.declared), uintField(uint64(port.position)))
		}
		fields = append(fields, boolField(boundary.hasVararg), bytesField(boundary.vararg.id), bytesField(boundary.vararg.cell), uintField(uint64(len(boundary.captures))))
		for _, capture := range boundary.captures {
			fields = append(fields,
				bytesField(capture.id), bytesField(capture.inner), bytesField(capture.outer), bytesField(capture.innerBody), bytesField(capture.outerBody), uintField(uint64(capture.position)),
			)
		}
		fields = append(fields, uintField(uint64(len(boundary.outcomes))))
		for _, outcome := range boundary.outcomes {
			fields = append(fields, bytesField(outcome))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.callTargets))))
	for _, target := range artifact.callTargets {
		fields = append(fields, bytesField(target.allocation), bytesField(target.body), bytesField(target.context), bytesField(target.function), bytesField(target.formal))
	}
	fields = append(fields, uintField(uint64(len(artifact.boundaries))))
	for _, row := range artifact.boundaries {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.owner), uintField(uint64(row.position)), boolField(row.eligible))
	}
	fields = append(fields, uintField(uint64(len(artifact.outcomes))))
	for _, outcome := range artifact.outcomes {
		fields = append(fields,
			bytesField(outcome.id), bytesField(outcome.body), uintField(uint64(outcome.kind)),
			boolField(outcome.hasTarget), bytesField(outcome.target),
			boolField(outcome.hasPropagation), bytesField(outcome.propagation),
			uintField(uint64(outcome.returnStart)), uintField(uint64(outcome.returnEnd)), uintField(uint64(len(outcome.points))),
		)
		for _, point := range outcome.points {
			fields = append(fields, bytesField(point))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.returnValues))))
	for _, value := range artifact.returnValues {
		fields = append(fields, bytesField(value.id))
	}
	fields = append(fields, uintField(uint64(len(artifact.occurrences))))
	for _, row := range artifact.occurrences {
		fields = append(fields, uintField(uint64(row.kind)), bytesField(row.id), bytesField(row.body), uintField(row.code), uintField(uint64(len(row.points))))
		for _, point := range row.points {
			fields = append(fields, bytesField(point))
		}
		fields = append(fields, uintField(uint64(len(row.inputs))))
		for _, input := range row.inputs {
			fields = append(fields, bytesField(input))
		}
		fields = append(fields, uintField(uint64(row.literalFamily)), boolField(row.literalOK), uintField(uint64(row.literal.Kind)), boolField(row.literal.Bool), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits), field{bytes: []byte(row.literal.String), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.exactScalarSummaries))))
	for _, row := range artifact.exactScalarSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.subject), bytesField(row.body),
			uintField(uint64(row.role)), uintField(uint64(row.literal.Kind)), uintField(uint64(row.literal.Integer)), uintField(row.literal.FloatBits))
	}
	fields = append(fields, uintField(uint64(len(artifact.arithmeticSummaries))))
	for _, row := range artifact.arithmeticSummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), uintField(uint64(row.op)),
			uintField(uint64(row.left)), uintField(uint64(row.right)), uintField(uint64(row.result)), uintField(uint64(row.divisor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.unarySummaries))))
	for _, row := range artifact.unarySummaries {
		fields = append(fields, bytesField(row.id), bytesField(row.occurrence), bytesField(row.body), bytesField(row.point), uintField(uint64(row.op)),
			uintField(uint64(row.operand)), uintField(uint64(row.result)))
	}
	fields = append(fields, uintField(uint64(len(artifact.heapAllocations))))
	for _, allocation := range artifact.heapAllocations {
		fields = append(fields, bytesField(allocation.id), uintField(uint64(allocation.role)), uintField(uint64(allocation.form)), bytesField(allocation.rootSpan), uintField(uint64(len(allocation.fields))))
		for _, field := range allocation.fields {
			fields = append(fields, bytesField(field.id), uintField(uint64(field.kind)), bytesField(field.fieldSpan), bytesField(field.selectorSpan), bytesField(field.valuesSpan), bytesField(field.valuesID), uintField(uint64(field.width)), boolField(field.finalOpen), boolField(field.sharesFirstValueCell), uintField(uint64(field.normalized)), boolField(field.normalizedOK))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.heapIndexes))))
	for _, access := range artifact.heapIndexes {
		fields = append(fields, bytesField(access.id), boolField(access.read), bytesField(access.baseSpan), bytesField(access.resultSpan), bytesField(access.keySpan), uintField(uint64(access.lensKind)), uintField(uint64(access.exactKey)), bytesField(access.valuesSpan), bytesField(access.valuesID), uintField(uint64(access.position+1)))
	}
	fields = append(fields, uintField(diagnosticLawVersion), uintField(uint64(len(artifact.diagnosticObservations))))
	for _, row := range artifact.diagnosticObservations {
		fields = append(fields,
			bytesField(row.id), uintField(uint64(row.kind)),
			field{bytes: []byte(row.location.File), kind: fieldBytes}, uintField(uint64(row.location.StartLine)),
			uintField(uint64(row.location.StartCol)), uintField(uint64(row.location.EndLine)), uintField(uint64(row.location.EndCol)),
		)
		switch row.kind {
		case structure.DiagnosticObservationBranchCondition:
			fields = append(fields, bytesField(row.branch.decision), bytesField(row.branch.value), uintField(uint64(len(row.branch.points))))
			for _, point := range row.branch.points {
				fields = append(fields, bytesField(point))
			}
		case structure.DiagnosticObservationTypeReferenceUnresolved:
			fields = append(fields, bytesField(row.unresolved.reference), bytesField(row.unresolved.root), uintField(uint64(len(row.unresolved.path))))
			for _, component := range row.unresolved.path {
				fields = append(fields, field{bytes: []byte(component), kind: fieldBytes})
			}
		case structure.DiagnosticObservationValueReferenceUnresolved:
			fields = append(fields, bytesField(row.value.read), bytesField(row.value.cell), field{bytes: []byte(row.value.name), kind: fieldBytes})
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeArguments))))
	for _, row := range artifact.staticTypeArguments {
		fields = append(fields, bytesField(row.id), bytesField(row.call), bytesField(row.types), bytesField(row.reference), uintField(uint64(row.index)))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeValues))))
	for _, row := range artifact.staticTypeValues {
		fields = append(fields, bytesField(row.id), bytesField(row.body), bytesField(row.reference), bytesField(row.root), field{bytes: []byte(row.name), kind: fieldBytes})
	}
	fields = append(fields, uintField(uint64(len(artifact.staticTypeNodes))))
	for _, row := range artifact.staticTypeNodes {
		exact := row.exact
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), field{bytes: []byte(row.name), kind: fieldBytes}, uintField(uint64(row.key)), uintField(uint64(row.literal)), uintField(row.bits), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, boolField(row.flag), uintField(uint64(row.resolution)), uintField(uint64(row.assertParam)), bytesField(row.declaration), bytesField(row.operand), bytesField(row.scope), bytesField(row.assertionNarrow), uintField(uint64(row.assertionCoordinate[0])), uintField(uint64(row.assertionCoordinate[1])), uintField(uint64(row.assertionCoordinate[2])), uintField(uint64(row.assertionCoordinate[3])), bytesField(row.typeFunctionVariadic), uintField(uint64(len(row.aliasParams))))
		for _, child := range row.aliasParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceExtends))))
		for _, child := range row.interfaceExtends {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.interfaceMemberTypes))))
		for _, child := range row.interfaceMemberTypes {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionTypeParams))))
		for _, child := range row.typeFunctionTypeParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionParams))))
		for _, child := range row.typeFunctionParams {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.typeFunctionReturns))))
		for _, child := range row.typeFunctionReturns {
			fields = append(fields, bytesField(child))
		}
		fields = append(fields, uintField(uint64(len(row.fieldKeys))))
		for index, key := range row.fieldKeys {
			fields = append(fields, uintField(uint64(key)))
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
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes}, boolField(optional), boolField(readonly))
		}
		fields = append(fields, uintField(uint64(len(row.keys))))
		for _, key := range row.keys {
			fields = append(fields, uintField(uint64(key)))
		}
		for index := range row.keys {
			text := ""
			if index < len(row.texts) {
				text = row.texts[index]
			}
			fields = append(fields, field{bytes: []byte(text), kind: fieldBytes})
			optional := false
			if index < len(row.optional) {
				optional = row.optional[index]
			}
			memberKind := uint8(0)
			if index < len(row.memberKinds) {
				memberKind = row.memberKinds[index]
			}
			fields = append(fields, boolField(optional), uintField(uint64(memberKind)))
		}
		fields = append(fields, uintField(uint64(len(row.segments))))
		for _, segment := range row.segments {
			fields = append(fields, uintField(uint64(segment)))
		}
		fields = append(fields, boolField(row.returnsKnown))
		fields = append(fields, uintField(uint64(len(row.sourceKeys))))
		for _, key := range row.sourceKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.canonicalKeys))))
		for _, key := range row.canonicalKeys {
			fields = append(fields, uintField(uint64(key)))
		}
		fields = append(fields, uintField(uint64(len(row.children))))
		for _, child := range row.children {
			fields = append(fields, bytesField(child))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.staticExpressions))))
	for _, row := range artifact.staticExpressions {
		fields = append(fields, bytesField(row.id), bytesField(row.reference), bytesField(row.owner))
	}
	fields = append(fields, uintField(uint64(len(artifact.staticInputs))))
	for _, row := range artifact.staticInputs {
		exact := row.literal
		fields = append(fields, bytesField(row.id), bytesField(row.owner), uintField(uint64(row.kind)), uintField(uint64(row.operandKind)), bytesField(row.expression), bytesField(row.source), bytesField(row.target), bytesField(row.operand), bytesField(row.frontier), bytesField(row.operandReference), bytesField(row.operandSubject), bytesField(row.operandBody), uintField(uint64(exact.Kind)), boolField(exact.Bool), uintField(uint64(exact.Integer)), uintField(exact.FloatBits), field{bytes: []byte(exact.String), kind: fieldBytes}, uintField(uint64(row.cursor)))
	}
	fields = append(fields, uintField(uint64(len(artifact.environment))))
	for _, edge := range artifact.environment {
		fields = append(fields,
			bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), bytesField(edge.route),
			uintField(uint64(edge.arm)), bytesField(edge.guard), bytesField(edge.decision), bytesField(edge.condition), boolField(edge.guarded), boolField(edge.truth),
			bytesField(edge.component), bytesField(edge.mu), boolField(edge.hasMu),
			bytesField(edge.reset), boolField(edge.hasReset), uintField(uint64(len(edge.resets))),
		)
		for _, reset := range edge.resets {
			fields = append(fields, bytesField(reset))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.localTransfers))))
	for _, edge := range artifact.localTransfers {
		fields = append(fields, bytesField(edge.id), bytesField(edge.from), bytesField(edge.to), boolField(edge.full), uintField(uint64(len(edge.roles))))
		for _, role := range edge.roles {
			fields = append(fields, uintField(uint64(role)))
		}
	}
	for roleIndex := 0; roleIndex < MountedRuleRoleCount(); roleIndex++ {
		role, roleOK := MountedRuleRoleAt(roleIndex)
		if !roleOK {
			continue
		}
		rows := artifact.ruleOccurrences[role]
		fields = append(fields, uintField(uint64(role)), uintField(uint64(len(rows))))
		for _, row := range rows {
			fields = append(fields,
				uintField(uint64(row.occurrence)), bytesField(row.point), bytesField(row.input),
				uintField(uint64(row.stage)), uintField(uint64(row.inputKind)), bytesField(row.route),
			)
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.regions))))
	for _, region := range artifact.regions {
		fields = append(fields,
			bytesField(region.id), bytesField(region.head), bytesField(region.sourceHead), bytesField(region.parent), boolField(region.cyclic),
			uintField(uint64(len(region.members))),
		)
		for _, member := range region.members {
			fields = append(fields, bytesField(member))
		}
	}
	fields = append(fields, uintField(uint64(len(artifact.events))))
	for _, event := range artifact.events {
		fields = append(fields, uintField(uint64(event.kind)), bytesField(event.region), bytesField(event.point))
	}
	return digest(artifactIDDomain, artifactFormat, fields...)
}
