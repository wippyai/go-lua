package analysis

import (
	"github.com/wippyai/go-lua/analysis/identity"
	programartifact "github.com/wippyai/go-lua/analysis/program/artifact"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// artifactResultReceipt is the compile-time, mount-qualified geometry needed
// to project query rows.  It is deliberately independent of Link and of the
// live ProgramArtifact handles: Solve copies only these detached rows before
// applying query observations.
type artifactResultReceipt struct {
	source             identity.ContentID
	bodies             []artifactResultBodyReceipt
	values             []identity.ContentID
	branchObservations []compiledObservation
	staticObservations []compiledObservation
	nativeScalars      []compiledNativeScalarSource
	nativeArithmetics  []compiledNativeArithmeticSummary
	nativeUnaries      []compiledNativeUnarySummary
	pointBodies        map[artifactResultPoint][]int
	pointObservations  map[artifactResultPoint][]compiledObservation
}

type compiledNativeArithmeticSummary struct {
	mount, artifact, proof, occurrence, body, point, span identity.ContentID
	op                                                    flowkind.BinaryOp
	left, right, result                                   programartifact.NumericRepresentation
	divisor                                               programartifact.ArithmeticDivisorProperty
}

type compiledNativeUnarySummary struct {
	mount, artifact, proof, occurrence, body, point, span identity.ContentID
	op                                                    flowkind.UnaryOp
	operand, result                                       programartifact.NumericRepresentation
}

func (summary compiledNativeUnarySummary) valid() bool {
	return summary.mount.Available() && summary.artifact.Available() && summary.proof.Available() &&
		summary.occurrence.Available() && summary.body.Available() && summary.point.Available() && summary.span.Available() &&
		summary.proof != summary.occurrence && summary.op == flowkind.UnaryNeg && summary.operand.Valid() && summary.result.Valid() &&
		summary.operand == summary.result
}

func (summary compiledNativeArithmeticSummary) valid() bool {
	return summary.mount.Available() && summary.artifact.Available() && summary.proof.Available() &&
		summary.occurrence.Available() && summary.body.Available() && summary.point.Available() && summary.span.Available() &&
		summary.proof != summary.occurrence && summary.op >= flowkind.BinaryAdd && summary.op <= flowkind.BinaryPow &&
		summary.left.Valid() && summary.right.Valid() && summary.result.Valid() && summary.divisor.Valid() &&
		(summary.divisor == programartifact.ArithmeticDivisorNone || summary.op == flowkind.BinaryIDiv)
}

type compiledNativeScalarSourceKind uint8

const (
	compiledNativeScalarSourceProgramSummary compiledNativeScalarSourceKind = iota + 1
)

// compiledNativeScalarSource is ProgramArtifact's exact arithmetic-use
// summary after Link adds only mount identity. It never publishes every
// authored literal globally: only a Program-authenticated exact operand or
// result reaches this detached projection.
type compiledNativeScalarSource struct {
	kind       compiledNativeScalarSourceKind
	mount      identity.ContentID
	artifact   identity.ContentID
	proof      identity.ContentID
	occurrence identity.ContentID
	body       identity.ContentID
	point      identity.ContentID
	span       identity.ContentID
	family     keyspace.Family
	literal    keyspace.LiteralValue
}

func (source compiledNativeScalarSource) valid() bool {
	if !source.mount.Available() || !source.artifact.Available() || !source.proof.Available() ||
		!source.occurrence.Available() || !source.body.Available() || !source.point.Available() || !source.span.Available() {
		return false
	}
	switch source.kind {
	case compiledNativeScalarSourceProgramSummary:
		if source.proof == source.occurrence ||
			(source.literal.Kind != keyspace.LiteralInteger && source.literal.Kind != keyspace.LiteralFloat) {
			return false
		}
	default:
		return false
	}
	switch source.family {
	case keyspace.FamilyNil:
		return source.literal == (keyspace.LiteralValue{})
	case keyspace.FamilyBool:
		return source.literal.Kind == keyspace.LiteralBool
	case keyspace.FamilyInteger:
		return source.literal.Kind == keyspace.LiteralInteger
	case keyspace.FamilyFloat:
		return source.literal.Kind == keyspace.LiteralFloat
	case keyspace.FamilyString:
		return source.literal.Kind == keyspace.LiteralString
	default:
		return false
	}
}

type artifactResultBodyReceipt struct {
	key   artifactResultBody
	id    identity.ContentID
	roots []resultRoot
}

func compileArtifactResultReceipt(
	sourceID identity.ContentID,
	mounts []mountedProgramArtifact,
	coordinates []compiledValueCoordinate,
	observations []compiledObservation,
) (*artifactResultReceipt, bool) {
	if !sourceID.Available() || len(mounts) == 0 || len(coordinates) == 0 {
		return nil, false
	}
	receipt := &artifactResultReceipt{
		source:             sourceID,
		bodies:             make([]artifactResultBodyReceipt, 0),
		values:             make([]identity.ContentID, len(coordinates)),
		branchObservations: make([]compiledObservation, 0, len(observations)),
		staticObservations: make([]compiledObservation, 0, len(observations)),
		nativeScalars:      make([]compiledNativeScalarSource, 0),
		nativeArithmetics:  make([]compiledNativeArithmeticSummary, 0),
		nativeUnaries:      make([]compiledNativeUnarySummary, 0),
		pointBodies:        make(map[artifactResultPoint][]int),
		pointObservations:  make(map[artifactResultPoint][]compiledObservation),
	}
	for _, observation := range observations {
		if !observation.available() {
			return nil, false
		}
		copy := observation
		copy.points = append([]identity.ContentID(nil), observation.points...)
		copy.producers = append([]compiledObservationProducer(nil), observation.producers...)
		copy.path = append([]string(nil), observation.path...)
		switch observation.kind {
		case programartifact.DiagnosticObservationBranchCondition:
			receipt.branchObservations = append(receipt.branchObservations, copy)
		case programartifact.DiagnosticObservationTypeReferenceUnresolved, programartifact.DiagnosticObservationValueReferenceUnresolved:
			receipt.staticObservations = append(receipt.staticObservations, copy)
		default:
			return nil, false
		}
	}
	// The public Result contract projects every declared Value from each Body,
	// but Value identity is global to this Result. Keep exactly one ordered axis;
	// individual Body results retain only a canonical presence bitmap.
	artifactIDs := make(map[identity.ContentID]identity.ContentID, len(mounts))
	bodyIndexes := make(map[artifactResultBody]int)
	for _, mount := range mounts {
		if mount.artifact == nil || !mount.artifact.Available() || !mount.moduleKey.Available() || !mount.artifact.ID().Available() {
			return nil, false
		}
		if _, duplicate := artifactIDs[mount.moduleKey]; duplicate {
			return nil, false
		}
		artifactIDs[mount.moduleKey] = mount.artifact.ID()
		localBodies := make(map[identity.ContentID]int)
		for bodyIndex := 0; bodyIndex < mount.artifact.BodyCount(); bodyIndex++ {
			body, bodyOK := mount.artifact.BodyAt(bodyIndex)
			if !bodyOK || !body.Available() || !body.ID().Available() {
				return nil, false
			}
			key := artifactResultBody{mount: mount.moduleKey, body: body.ID()}
			id, idOK := mountedResultID("body", mount.moduleKey, mount.artifact.ID(), body.ID())
			if !idOK {
				return nil, false
			}
			if _, duplicate := localBodies[body.ID()]; duplicate {
				return nil, false
			}
			if _, duplicate := bodyIndexes[key]; duplicate {
				return nil, false
			}
			localBodies[body.ID()] = len(receipt.bodies)
			bodyIndexes[key] = len(receipt.bodies)
			roots := make([]resultRoot, body.RootCount())
			seenRoots := make(map[identity.ContentID]struct{}, len(roots))
			for rootIndex := range roots {
				root, rootOK := body.RootAt(rootIndex)
				if !rootOK || !root.Available() || !root.ID().Available() || root.Family() == keyspace.FamilyInvalid {
					return nil, false
				}
				rootID, rootIDOK := mountedResultID("root", mount.moduleKey, mount.artifact.ID(), root.ID())
				if !rootIDOK {
					return nil, false
				}
				if _, duplicate := seenRoots[rootID]; duplicate {
					return nil, false
				}
				seenRoots[rootID] = struct{}{}
				roots[rootIndex] = resultRoot{id: rootID, family: root.Family()}
			}
			receipt.bodies = append(receipt.bodies, artifactResultBodyReceipt{key: key, id: id, roots: roots})
		}
		for occurrenceIndex := 0; occurrenceIndex < mount.artifact.OccurrenceCount(); occurrenceIndex++ {
			occurrence, occurrenceOK := mount.artifact.OccurrenceAt(occurrenceIndex)
			if !occurrenceOK || !occurrence.Available() {
				return nil, false
			}
			bodyID, bodyOK := occurrence.BodyID()
			if !bodyOK {
				continue
			}
			bodyIndex, bodyKnown := localBodies[bodyID]
			if !bodyKnown {
				return nil, false
			}
			for pointIndex := 0; pointIndex < occurrence.PointCount(); pointIndex++ {
				point, pointOK := occurrence.PointAt(pointIndex)
				if !pointOK || !point.Available() {
					return nil, false
				}
				pointKey := artifactResultPoint{mount: mount.moduleKey, point: point}
				receipt.pointBodies[pointKey] = appendUniqueInt(receipt.pointBodies[pointKey], bodyIndex)
			}
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.ExactScalarSummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.ExactScalarSummaryAt(summaryIndex)
			literal, literalOK := summary.Literal()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			if !summaryOK || !literalOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID {
				return nil, false
			}
			point, pointOK := exactNativeScalarRulePoint(mount.artifact, programartifact.RuleRoleValueBinaryArithmetic, summary.OccurrenceID())
			family := keyspace.FamilyInvalid
			switch literal.Kind {
			case keyspace.LiteralInteger:
				family = keyspace.FamilyInteger
			case keyspace.LiteralFloat:
				family = keyspace.FamilyFloat
			}
			source := compiledNativeScalarSource{
				kind: compiledNativeScalarSourceProgramSummary, mount: mount.moduleKey, artifact: mount.artifact.ID(),
				proof: summary.ID(), occurrence: summary.OccurrenceID(), body: bodyID, point: point, span: summary.SubjectID(),
				family: family, literal: literal,
			}
			if !pointOK || !source.valid() {
				return nil, false
			}
			receipt.nativeScalars = append(receipt.nativeScalars, source)
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.ArithmeticSummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.ArithmeticSummaryAt(summaryIndex)
			left, right, result, representationsOK := summary.Representations()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceBinaryArithmetic, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			point, pointOK := exactNativeScalarRulePoint(mount.artifact, programartifact.RuleRoleValueBinaryArithmetic, summary.OccurrenceID())
			compiled := compiledNativeArithmeticSummary{
				mount: mount.moduleKey, artifact: mount.artifact.ID(), proof: summary.ID(), occurrence: summary.OccurrenceID(),
				body: bodyID, point: point, span: occurrence.ID(), op: summary.Operator(), left: left, right: right, result: result,
				divisor: summary.DivisorProperty(),
			}
			if !summaryOK || !representationsOK || !occurrenceOK || !bodyOK || !pointOK || summary.BodyPathID() != bodyID || !compiled.valid() {
				return nil, false
			}
			receipt.nativeArithmetics = append(receipt.nativeArithmetics, compiled)
		}
		for summaryIndex := 0; summaryIndex < mount.artifact.UnarySummaryCount(); summaryIndex++ {
			summary, summaryOK := mount.artifact.UnarySummaryAt(summaryIndex)
			operand, result, representationsOK := summary.Representations()
			occurrence, occurrenceOK := mount.artifact.OccurrenceForID(programartifact.OccurrenceUnary, summary.OccurrenceID())
			bodyID, bodyOK := occurrence.BodyID()
			compiled := compiledNativeUnarySummary{
				mount: mount.moduleKey, artifact: mount.artifact.ID(), proof: summary.ID(), occurrence: summary.OccurrenceID(),
				body: bodyID, point: summary.OutputPointID(), span: occurrence.ID(), op: summary.Operator(), operand: operand, result: result,
			}
			if !summaryOK || !representationsOK || !occurrenceOK || !bodyOK || summary.BodyPathID() != bodyID || !compiled.valid() {
				return nil, false
			}
			receipt.nativeUnaries = append(receipt.nativeUnaries, compiled)
		}
	}
	for index, coordinate := range coordinates {
		if !coordinate.id.Available() || !coordinate.mount.Available() {
			return nil, false
		}
		artifactID, artifactOK := artifactIDs[coordinate.mount]
		id, idOK := mountedResultID("value", coordinate.mount, artifactID, coordinate.id)
		if !artifactOK || !idOK {
			return nil, false
		}
		receipt.values[index] = id
	}
	for _, observation := range receipt.branchObservations {
		if !observation.id.Available() || !observation.mount.Available() || !observation.artifact.Available() ||
			!observation.local.Available() || len(observation.producers) == 0 || uint64(observation.valueIndex) >= uint64(len(coordinates)) ||
			observation.kind != programartifact.DiagnosticObservationBranchCondition {
			return nil, false
		}
		artifactID, artifactOK := artifactIDs[observation.mount]
		if !artifactOK || artifactID != observation.artifact {
			return nil, false
		}
		coordinate := coordinates[observation.valueIndex]
		if coordinate.mount != observation.mount {
			return nil, false
		}
		seenPoints := make(map[identity.ContentID]struct{}, len(observation.points))
		for _, point := range observation.points {
			if !point.Available() {
				return nil, false
			}
			if _, duplicate := seenPoints[point]; duplicate {
				return nil, false
			}
			seenPoints[point] = struct{}{}
			key := artifactResultPoint{mount: observation.mount, point: point}
			receipt.pointObservations[key] = append(receipt.pointObservations[key], observation)
		}
		seenAnchors := make(map[identity.ContentID]struct{}, len(observation.producers))
		seenExecution := make(map[identity.ContentID]struct{}, len(observation.producers))
		for _, producer := range observation.producers {
			if !mountedDiagnosticRuleRole(producer.role) || !producer.occurrence.Available() || !producer.point.Available() || !producer.anchor.Available() {
				return nil, false
			}
			if _, known := seenPoints[producer.anchor]; !known {
				return nil, false
			}
			if _, duplicate := seenAnchors[producer.anchor]; duplicate {
				return nil, false
			}
			if _, duplicate := seenExecution[producer.point]; duplicate {
				return nil, false
			}
			seenAnchors[producer.anchor] = struct{}{}
			seenExecution[producer.point] = struct{}{}
		}
		if len(seenAnchors) != len(seenPoints) {
			return nil, false
		}
	}
	for _, observation := range receipt.staticObservations {
		if !observation.available() {
			return nil, false
		}
		switch observation.kind {
		case programartifact.DiagnosticObservationTypeReferenceUnresolved:
			if !observation.reference.Available() || len(observation.path) == 0 {
				return nil, false
			}
		case programartifact.DiagnosticObservationValueReferenceUnresolved:
			if !observation.read.Available() || !observation.cell.Available() || observation.name == "" {
				return nil, false
			}
		default:
			return nil, false
		}
		artifactID, artifactOK := artifactIDs[observation.mount]
		if !artifactOK || artifactID != observation.artifact {
			return nil, false
		}
	}
	return receipt, receipt.valid()
}

func exactNativeScalarRulePoint(artifact *programartifact.Artifact, role programartifact.RuleRole, occurrence identity.ContentID) (identity.ContentID, bool) {
	if artifact == nil || !artifact.Available() || !occurrence.Available() {
		return identity.ContentID{}, false
	}
	var point identity.ContentID
	found := false
	for index := 0; index < artifact.RuleOccurrenceCount(role); index++ {
		row, rowOK := artifact.RuleOccurrenceAt(role, index)
		output, outputOK := row.OutputSemanticID()
		candidate, pointOK := row.PointAt(0)
		if !rowOK || !outputOK || !pointOK {
			return identity.ContentID{}, false
		}
		if row.ID() != occurrence || output != occurrence {
			continue
		}
		if found || row.Stage() != programartifact.RuleStageLocal {
			return identity.ContentID{}, false
		}
		point, found = candidate, true
	}
	return point, found && point.Available()
}

func (receipt *artifactResultReceipt) valid() bool {
	if receipt == nil || !receipt.source.Available() || len(receipt.bodies) == 0 || len(receipt.values) == 0 || receipt.pointBodies == nil || receipt.pointObservations == nil || receipt.nativeScalars == nil || receipt.nativeArithmetics == nil || receipt.nativeUnaries == nil {
		return false
	}
	for _, source := range receipt.nativeScalars {
		if !source.valid() {
			return false
		}
	}
	for _, summary := range receipt.nativeArithmetics {
		if !summary.valid() {
			return false
		}
	}
	for _, summary := range receipt.nativeUnaries {
		if !summary.valid() {
			return false
		}
	}
	return true
}
