package result

import (
	"encoding/binary"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/program"
	"github.com/wippyai/go-lua/analysis/schema/programmount"
	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/analysis/snapshot"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// buildNativeBranchPublication is the sole post-convergence owner for the
// initial native Value families. It consumes only Program-issued mounted
// branch geometry and Engine-authenticated Value observations. Manifest rows
// and diagnostic policy never enter this producer.
func buildNativeBranchPublication(
	geometry Geometry,
	mounts []programmount.MountedArtifact,
	selected []anadiag.ObservationKey,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
) ([]nativePublicationRow, bool) {
	if !geometry.Valid() || published == nil || !published.Published() || !observationPlan.Available() {
		return nil, false
	}
	type observedKey struct {
		point   Point
		context identity.ContentID
	}
	observed := make(map[observedKey]valuedomain.ValueSummaryObservation, len(selected))
	for _, selectedObservation := range selected {
		key := Point{Mount: selectedObservation.Mount, Point: selectedObservation.Point}
		if !key.Mount.Available() || !key.Point.Available() || !selectedObservation.Context.Available() {
			return nil, false
		}
		lookup := observedKey{point: key, context: selectedObservation.Context}
		if _, duplicate := observed[lookup]; duplicate {
			return nil, false
		}
		observationID := selectedObservation.Key
		observation, readable := publishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, observationID)
		if !observationID.Available() || !readable || !validNativeValueSummary(observation) {
			return nil, false
		}
		observed[lookup] = observation
	}
	rows := make([]nativePublicationRow, 0)
	byID := make(map[identity.ContentID]struct{})
	if !appendNativeArtifactSummaryRows(&rows, byID, mounts) {
		return nil, false
	}
	expected := make(map[observedKey]struct{})
	for _, subject := range geometry.BranchObservations {
		if subject.Kind != structure.DiagnosticObservationBranchCondition || !subject.Available() {
			return nil, false
		}
		coordinate, coordinateOK := geometry.valueResultID(subject.Mount, subject.Branch.ValueID)
		if !coordinateOK {
			return nil, false
		}
		// One projected branch row may be read under several exact actor
		// Contexts. The selected observation keys are the owner-issued context
		// inventory; process every lane rather than choosing a representative.
		contexts := make(map[identity.ContentID]struct{})
		for _, point := range subject.Points {
			key := Point{Mount: subject.Mount, Point: point}
			if _, bodyOK := nativePublicationBodyAt(geometry, key); !bodyOK {
				return nil, false
			}
			for candidate := range observed {
				if candidate.point == key {
					contexts[candidate.context] = struct{}{}
				}
			}
		}
		if len(contexts) == 0 {
			return nil, false
		}
		for context := range contexts {
			truth := valuedomain.TruthNone
			complete := true
			var subjectBody identity.ContentID
			for _, point := range subject.Points {
				key := Point{Mount: subject.Mount, Point: point}
				expected[observedKey{point: key, context: context}] = struct{}{}
				body, bodyOK := nativePublicationBodyAt(geometry, key)
				if !bodyOK || subjectBody.Available() && subjectBody != body {
					return nil, false
				}
				subjectBody = body
				observation, ok := observed[observedKey{point: key, context: context}]
				if !ok {
					return nil, false
				}
				if observation.Rows == 0 {
					complete = false
					continue
				}
				_, present, valueValid := observation.ValueAtID(subject.Branch.ValueID)
				if !valueValid {
					return nil, false
				}
				if !present {
					complete = false
					continue
				}
				// A branch observation authenticates the condition coordinate. Other
				// cells share its point snapshot but are not native-publication uses;
				// publishing them globally leaks path-local literals across merges.
				if !appendNativeScalarRows(&rows, byID, observation, subject.Branch.ValueID, coordinate, subject, subjectBody, point, context) {
					return nil, false
				}
				pointTruth, truthPresent, truthValid := observation.TruthinessAtID(subject.Branch.ValueID)
				if !truthValid || !truthPresent {
					return nil, false
				}
				if pointTruth == valuedomain.TruthNone {
					complete = false
					continue
				}
				truth |= pointTruth
			}
			if !appendNativeBranchRows(&rows, byID, subject, subjectBody, truth, complete, context) {
				return nil, false
			}
		}
	}
	if len(observed) != len(expected) {
		return nil, false
	}
	return rows, true
}

func appendNativeArithmeticRows(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	summary programschema.ArithmeticSummary,
	mount, artifact, span, body, point identity.ContentID,
) bool {
	left, right, resultRepresentation, representationsOK := summary.Representations()
	op := flowkind.BinaryOp(summary.Operator())
	overflow, overflowOK := valuedomain.BinaryNumericOverflow(op, left, right)
	divisor, divisorOK := nativeDivisorPropertyOf(summary.DivisorProperty())
	evidence, evidenceOK := nativeEvidencePoints(point)
	if !validNativeArithmeticSummary(summary, mount, artifact, body, point, span) || !representationsOK || !overflowOK || !divisorOK || !evidenceOK {
		return false
	}
	representation := nativePublicationContent{
		exact:          resultRepresentation != programschema.NumericRepresentationNumber,
		representation: resultRepresentation,
		left:           left,
		right:          right,
		binary:         op,
		overflow:       overflow,
		points:         evidence,
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyRepresentation, summary, mount, artifact, span, body, point, representation) {
		return false
	}
	operator := nativePublicationContent{
		representation: resultRepresentation,
		left:           left,
		right:          right,
		binary:         op,
		overflow:       overflow,
		divisor:        divisor,
		points:         evidence,
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyScalarOperator, summary, mount, artifact, span, body, point, operator) {
		return false
	}
	// A float division carries no integer divisor obligation at all, so the
	// divisor row states that rather than leaving the question unpublished.
	if op == flowkind.BinaryDiv {
		divisor = NativeDivisorPropertyNotApplicable
	}
	if !divisor.Available() {
		return true
	}
	return appendNativeArithmeticRow(rows, seen, nativePublicationFamilyDivisorProperty, summary, mount, artifact, span, body, point,
		nativePublicationContent{binary: op, divisor: divisor, points: evidence})
}

func appendNativeArithmeticRow(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	family nativePublicationFamily,
	summary programschema.ArithmeticSummary,
	mount, artifact, span, body, point identity.ContentID,
	content nativePublicationContent,
) bool {
	semantic, semanticOK := family.semanticID()
	if !validNativeArithmeticSummary(summary, mount, artifact, body, point, span) || !semanticOK || !content.valid(family) {
		return false
	}
	proof, occurrence := summary.ID(), summary.OccurrenceID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), content: content,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
	return appendNativePublicationRow(rows, seen, row)
}

func appendNativeUnaryRows(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	summary programschema.UnarySummary,
	mount, artifact, body identity.ContentID,
) bool {
	operandRepresentation, resultRepresentation, representationsOK := summary.Representations()
	op := flowkind.UnaryOp(summary.Operator())
	overflow, overflowOK := valuedomain.UnaryNumericOverflow(op, operandRepresentation)
	point, span := summary.OutputPointID(), summary.OccurrenceID()
	evidence, evidenceOK := nativeEvidencePoints(point)
	if !validNativeUnarySummary(summary, mount, artifact, body) || !representationsOK || !overflowOK || !evidenceOK {
		return false
	}
	content := nativePublicationContent{
		exact:          resultRepresentation != programschema.NumericRepresentationNumber,
		representation: resultRepresentation,
		operand:        operandRepresentation,
		unary:          op,
		overflow:       overflow,
		points:         evidence,
	}
	semantic, semanticOK := nativePublicationFamilyRepresentation.semanticID()
	if !semanticOK || !content.valid(nativePublicationFamilyRepresentation) {
		return false
	}
	proof, occurrence := summary.ID(), summary.OccurrenceID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: nativePublicationFamilyRepresentation, trust: NativePublicationTrustProven,
		key: nativePublicationFamilyRepresentation.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), content: content,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
	return appendNativePublicationRow(rows, seen, row)
}

// appendNativePublicationRow seals one row: it derives the row's identity from
// its typed content, admits it once, and rejects a row that does not satisfy
// its own validity law.
func appendNativePublicationRow(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, row nativePublicationRow) bool {
	id, idOK := nativePublicationRowID(row)
	if !idOK {
		return false
	}
	row.id = id
	if _, duplicate := seen[id]; duplicate {
		return true
	}
	if !row.valid() {
		return false
	}
	seen[id] = struct{}{}
	*rows = append(*rows, row)
	return true
}

func validNativeArithmeticSummary(summary programschema.ArithmeticSummary, mount, artifact, body, point, span identity.ContentID) bool {
	if !summary.Available() || !mount.Available() || !artifact.Available() || !body.Available() || !point.Available() || !span.Available() {
		return false
	}
	op := flowkind.BinaryOp(summary.Operator())
	left, right, result, representationsOK := summary.Representations()
	divisor := summary.DivisorProperty()
	return summary.ID() != summary.OccurrenceID() && summary.BodyPathID() == body &&
		flowkind.IsBinaryArithmetic(op) && representationsOK && left.Valid() && right.Valid() && result.Valid() && divisor.Valid() &&
		(divisor == programschema.ArithmeticDivisorNone || op == flowkind.BinaryIDiv)
}

func validNativeUnarySummary(summary programschema.UnarySummary, mount, artifact, body identity.ContentID) bool {
	if !summary.Available() || !mount.Available() || !artifact.Available() || !body.Available() {
		return false
	}
	operand, result, representationsOK := summary.Representations()
	return summary.ID() != summary.OccurrenceID() && summary.BodyPathID() == body && summary.OutputPointID().Available() &&
		flowkind.UnaryOp(summary.Operator()) == flowkind.UnaryNeg && representationsOK && operand.Valid() && result.Valid() && operand == result
}

func nativePublicationBodyAt(geometry Geometry, point Point) (identity.ContentID, bool) {
	if !geometry.Valid() {
		return identity.ContentID{}, false
	}
	indexes := geometry.PointBodies[point]
	if len(indexes) != 1 || indexes[0] < 0 || indexes[0] >= len(geometry.bodies) {
		return identity.ContentID{}, false
	}
	body := geometry.bodies[indexes[0]].key.body
	return body, body.Available()
}

func appendNativeStaticScalarRows(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	summary programschema.ExactScalarSummary,
	mount, artifact, body, point identity.ContentID,
) bool {
	literal, ok := nativeSummaryLiteral(summary, mount, artifact, body, point)
	scalar, scalarOK := nativeScalarRepresentationOf(literal.Kind)
	evidence, evidenceOK := nativeEvidencePoints(point)
	if !ok || !scalarOK || !evidenceOK {
		return false
	}
	if !appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyConstantValue, summary, mount, artifact, body, point,
		nativePublicationContent{literal: literal, scalar: scalar, points: evidence}) {
		return false
	}
	// The carrier row states the carrier alone: the constant itself is the
	// constant-value row's content, and one fact has one owner.
	return appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyRepresentation, summary, mount, artifact, body, point,
		nativePublicationContent{exact: true, scalar: scalar, points: evidence})
}

func appendNativeStaticScalarRow(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	family nativePublicationFamily,
	summary programschema.ExactScalarSummary,
	mount, artifact, body, point identity.ContentID,
	content nativePublicationContent,
) bool {
	semantic, semanticOK := family.semanticID()
	if _, ok := nativeSummaryLiteral(summary, mount, artifact, body, point); !ok || !semanticOK || !content.valid(family) {
		return false
	}
	proof, occurrence, span := summary.ID(), summary.OccurrenceID(), summary.SubjectID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), content: content,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
	return appendNativePublicationRow(rows, seen, row)
}

// nativeSummaryLiteral is the Program-issued exact scalar of one reusable
// summary. Every bit pattern a Lua number holds is a literal here, infinities
// and NaN included: the bits are the fact, and withholding them would withhold
// the whole publication over a value the analyzer proved exactly.
func nativeSummaryLiteral(summary programschema.ExactScalarSummary, mount, artifact, body, point identity.ContentID) (keyspace.LiteralValue, bool) {
	if !summary.Available() || !mount.Available() || !artifact.Available() || !body.Available() || !point.Available() ||
		summary.ID() == summary.OccurrenceID() || summary.BodyPathID() != body {
		return keyspace.LiteralValue{}, false
	}
	coldLiteral, literalOK := summary.Literal()
	if !literalOK || (coldLiteral.Kind != uint8(keyspace.LiteralInteger) && coldLiteral.Kind != uint8(keyspace.LiteralFloat)) {
		return keyspace.LiteralValue{}, false
	}
	return keyspace.LiteralValue{Kind: keyspace.LiteralKind(coldLiteral.Kind), Integer: coldLiteral.Integer, FloatBits: coldLiteral.FloatBits}, true
}

func validNativeValueSummary(observation valuedomain.ValueSummaryObservation) bool {
	return observation.Valid && observation.Rows <= 1 && len(observation.Values) != 0 && len(observation.Values) == len(observation.Present)
}

func appendNativeScalarRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, observation valuedomain.ValueSummaryObservation, valueID, coordinate identity.ContentID, subject anadiag.Observation, body, point, context identity.ContentID) bool {
	scalar, exact, valid := observation.ExactScalarAtID(valueID)
	if !valid {
		return false
	}
	if !exact {
		return true
	}
	representation, literal, ok := nativeExactScalarColumns(scalar)
	evidence, evidenceOK := nativeEvidencePoints(point)
	if !ok || !evidenceOK {
		return true
	}
	if !appendNativeBranchRow(rows, seen, nativePublicationFamilyConstantValue, subject, body, point, coordinate,
		nativePublicationContent{literal: literal, scalar: representation, points: evidence}, context) {
		return false
	}
	return appendNativeBranchRow(rows, seen, nativePublicationFamilyRepresentation, subject, body, point, coordinate,
		nativePublicationContent{exact: true, scalar: representation, points: evidence}, context)
}

// appendNativeBranchRows publishes the branch condition's verdict over its
// whole evidence set. The set is the row's content, so a condition folded over
// several points publishes all of them rather than one representative.
func appendNativeBranchRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, subject anadiag.Observation, body identity.ContentID, truth valuedomain.Truth, complete bool, context identity.ContentID) bool {
	class, partition, deadArm, deadArmProved := nativeBranchVerdict(truth, complete)
	evidence, evidenceOK := nativeEvidencePoints(subject.Points...)
	if !class.Available() || !partition.Available() || !evidenceOK || deadArmProved != deadArm.Available() {
		return false
	}
	// Branch rows are keyed by the exact Program-issued observation identity,
	// not by a caller-provided coordinate or rendered source string. The
	// anchor point is the observation's first issued point; the evidence set
	// the verdict was folded over is published as content.
	point, coordinate := subject.Points[0], subject.ID
	if !appendNativeBranchRow(rows, seen, nativePublicationFamilyTruthinessClass, subject, body, point, coordinate,
		nativePublicationContent{truthiness: class, points: evidence}, context) {
		return false
	}
	return appendNativeBranchRow(rows, seen, nativePublicationFamilyBranchPartition, subject, body, point, coordinate,
		nativePublicationContent{partition: partition, deadArm: deadArm, points: evidence}, context)
}

func appendNativeBranchRow(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, family nativePublicationFamily, subject anadiag.Observation, body, point, coordinate identity.ContentID, content nativePublicationContent, context identity.ContentID) bool {
	semantic, semanticOK := family.semanticID()
	span, spanOK := nativePublicationSpanID(subject.Location)
	if !semanticOK || !spanOK || !coordinate.Available() || !body.Available() || !point.Available() || !content.valid(family) {
		return false
	}
	if !context.Available() {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + coordinate.String() + "/" + point.String(), module: subject.Location.File,
		term: coordinate.String(), subject: coordinate.String(), occurrence: subject.ID.String(), content: content,
		provenance:   NativePublicationProvenance{context: context, mount: subject.Mount, artifact: subject.Artifact, local: coordinate, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
	return appendNativePublicationRow(rows, seen, row)
}

func validMountedDiagnosticSpan(span programsource.Span) bool {
	if span.File == "" || span.StartLine == 0 || span.StartCol == 0 {
		return false
	}
	_, ok := programsource.CoordinateFromParts(span.StartLine, span.StartCol, span.EndLine, span.EndCol)
	return ok
}

func nativePublicationSpanID(span programsource.Span) (identity.ContentID, bool) {
	if !validMountedDiagnosticSpan(span) {
		return identity.ContentID{}, false
	}
	var coordinates [16]byte
	binary.BigEndian.PutUint32(coordinates[0:4], span.StartLine)
	binary.BigEndian.PutUint32(coordinates[4:8], span.StartCol)
	binary.BigEndian.PutUint32(coordinates[8:12], span.EndLine)
	binary.BigEndian.PutUint32(coordinates[12:16], span.EndCol)
	return identity.DeriveContentID("analysis/native-publication/source-span/v1", []byte(span.File), coordinates[:])
}

// nativeExactScalarColumns projects one proved exact scalar onto the columns a
// native row publishes it as. Lua nil carries no literal and keeps its own
// carrier; every other exact scalar publishes both.
func nativeExactScalarColumns(scalar valuedomain.ExactScalar) (NativeScalarRepresentation, keyspace.LiteralValue, bool) {
	switch scalar.Kind() {
	case valuedomain.ExactScalarNil:
		return NativeScalarRepresentationNil, keyspace.LiteralValue{}, true
	case valuedomain.ExactScalarBoolean, valuedomain.ExactScalarLiteral:
		literal, literalOK := scalar.Literal()
		if !literalOK {
			return NativeScalarRepresentationInvalid, keyspace.LiteralValue{}, false
		}
		representation, representationOK := nativeScalarRepresentationOf(literal.Kind)
		return representation, literal, representationOK
	default:
		return NativeScalarRepresentationInvalid, keyspace.LiteralValue{}, false
	}
}
