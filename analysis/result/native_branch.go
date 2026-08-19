package result

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	anadiag "github.com/wippyai/go-lua/analysis/diagnostic"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/identity"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	programsource "github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/schema/cold"
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
	mounts []Mount,
	selected []anadiag.ObservationKey,
	schema *valuedomain.Schema,
	published *snapshot.Snapshot,
	observationPlan snapshot.QueryPlan[identity.ContentID, engine.Answer],
) (*nativePublicationReceipt, bool) {
	if !geometry.Valid() || schema == nil || published == nil || !published.Published() || !observationPlan.Available() {
		return nil, false
	}
	observed := make(map[Point]valuedomain.ValueSummaryObservation, len(selected))
	for _, selectedObservation := range selected {
		key := Point{Mount: selectedObservation.Mount, Point: selectedObservation.Point}
		if !key.Mount.Available() || !key.Point.Available() {
			return nil, false
		}
		if _, duplicate := observed[key]; duplicate {
			return nil, false
		}
		observationID := selectedObservation.Key
		observation, readable := publishedObservation[valuedomain.ValueSummaryObservation](published, observationPlan, observationID)
		if !observationID.Available() || !readable || !validNativeValueSummary(observation, len(geometry.values)) {
			return nil, false
		}
		observed[key] = observation
	}
	rows := make([]nativePublicationRow, 0)
	byID := make(map[identity.ContentID]struct{})
	if !appendNativeArtifactSummaryRows(&rows, byID, mounts) {
		return nil, false
	}
	expected := make(map[Point]struct{})
	for _, subject := range geometry.BranchObservations {
		if subject.Kind != structure.DiagnosticObservationBranchCondition || !subject.Available() || int(subject.ValueIndex) >= len(geometry.values) {
			return nil, false
		}
		truth := valuedomain.TruthNone
		complete := true
		var subjectBody identity.ContentID
		for _, point := range subject.Points {
			key := Point{Mount: subject.Mount, Point: point}
			expected[key] = struct{}{}
			body, bodyOK := nativePublicationBodyAt(geometry, key)
			if !bodyOK || subjectBody.Available() && subjectBody != body {
				return nil, false
			}
			subjectBody = body
			observation, ok := observed[key]
			if !ok {
				return nil, false
			}
			if observation.Rows == 0 {
				complete = false
				continue
			}
			condition := int(subject.ValueIndex)
			if !observation.Present[condition] {
				complete = false
				continue
			}
			// A branch observation authenticates the condition coordinate. Other
			// cells share its point snapshot but are not native-publication uses;
			// publishing them globally leaks path-local literals across merges.
			if !appendNativeScalarRows(&rows, byID, schema, observation.Values[condition], geometry.values[condition], subject, subjectBody, point) {
				return nil, false
			}
			pointTruth := schema.Truthiness(observation.Values[condition])
			if pointTruth == valuedomain.TruthNone {
				complete = false
				continue
			}
			truth |= pointTruth
		}
		if !appendNativeBranchRows(&rows, byID, subject, subjectBody, truth, complete) {
			return nil, false
		}
	}
	if len(observed) != len(expected) {
		return nil, false
	}
	return newNativePublicationReceipt(rows)
}

func appendNativeArithmeticRows(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	summary cold.ArithmeticSummary,
	mount, artifact, span, body, point identity.ContentID,
) bool {
	left, right, resultRepresentation, representationsOK := summary.Representations()
	op := flowkind.BinaryOp(summary.Operator())
	operator, operatorOK := nativeArithmeticOperator(op)
	overflow, overflowOK := valuedomain.BinaryNumericOverflow(op, left, right)
	divisor, divisorOK := nativeArithmeticDivisor(summary.DivisorProperty())
	leftName, leftOK := nativeNumericRepresentation(left)
	rightName, rightOK := nativeNumericRepresentation(right)
	resultName, resultOK := nativeNumericRepresentation(resultRepresentation)
	if !validNativeArithmeticSummary(summary, mount, artifact, body, point, span) || !representationsOK || !leftOK || !rightOK || !resultOK || !operatorOK || !overflowOK || !divisorOK {
		return false
	}
	representation := "representation=" + resultName + " left=" + leftName + " operator=" + operator + " overflow=" + overflow.String() + " result_representation=" + resultName + " right=" + rightName
	if resultRepresentation != cold.NumericRepresentationNumber {
		representation = "exact=true " + representation
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyRepresentation, summary, mount, artifact, span, body, point, representation) {
		return false
	}
	operatorValue := "class=number dispatch=primitive left=" + leftName + " operator=" + operator + " overflow=" + overflow.String() + " result=" + resultName + " right=" + rightName
	if divisor != "" {
		operatorValue += " divisor=" + divisor
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyScalarOperator, summary, mount, artifact, span, body, point, operatorValue) {
		return false
	}
	if op == flowkind.BinaryDiv {
		return appendNativeArithmeticRow(rows, seen, nativePublicationFamilyDivisorProperty, summary, mount, artifact, span, body, point, "divisor=not_applicable operator=div")
	}
	if divisor != "" {
		return appendNativeArithmeticRow(rows, seen, nativePublicationFamilyDivisorProperty, summary, mount, artifact, span, body, point, "divisor="+divisor+" operator="+operator)
	}
	return true
}

func appendNativeArithmeticRow(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	family nativePublicationFamily,
	summary cold.ArithmeticSummary,
	mount, artifact, span, body, point identity.ContentID,
	value string,
) bool {
	semantic, semanticOK := family.semanticID()
	if !validNativeArithmeticSummary(summary, mount, artifact, body, point, span) || !semanticOK || value == "" {
		return false
	}
	proof, occurrence := summary.ID(), summary.OccurrenceID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
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

func appendNativeUnaryRows(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	summary cold.UnarySummary,
	mount, artifact, body identity.ContentID,
) bool {
	operandRepresentation, resultRepresentation, representationsOK := summary.Representations()
	op := flowkind.UnaryOp(summary.Operator())
	operand, operandOK := nativeNumericRepresentation(operandRepresentation)
	result, resultOK := nativeNumericRepresentation(resultRepresentation)
	overflow, overflowOK := valuedomain.UnaryNumericOverflow(op, operandRepresentation)
	point, span := summary.OutputPointID(), summary.OccurrenceID()
	if !validNativeUnarySummary(summary, mount, artifact, body) || !representationsOK || !operandOK || !resultOK || !overflowOK {
		return false
	}
	value := "operator=unm overflow=" + overflow.String() + " representation=" + result + " result_representation=" + result + " operand_representation=" + operand
	if resultRepresentation != cold.NumericRepresentationNumber {
		value = "exact=true " + value
	}
	semantic, semanticOK := nativePublicationFamilyRepresentation.semanticID()
	if !semanticOK {
		return false
	}
	proof, occurrence := summary.ID(), summary.OccurrenceID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: nativePublicationFamilyRepresentation, trust: NativePublicationTrustProven,
		key: nativePublicationFamilyRepresentation.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
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

func validNativeArithmeticSummary(summary cold.ArithmeticSummary, mount, artifact, body, point, span identity.ContentID) bool {
	if !summary.Available() || !mount.Available() || !artifact.Available() || !body.Available() || !point.Available() || !span.Available() {
		return false
	}
	op := flowkind.BinaryOp(summary.Operator())
	left, right, result, representationsOK := summary.Representations()
	divisor := summary.DivisorProperty()
	return summary.ID() != summary.OccurrenceID() && summary.BodyPathID() == body &&
		flowkind.IsBinaryArithmetic(op) && representationsOK && left.Valid() && right.Valid() && result.Valid() && divisor.Valid() &&
		(divisor == cold.ArithmeticDivisorNone || op == flowkind.BinaryIDiv)
}

func validNativeUnarySummary(summary cold.UnarySummary, mount, artifact, body identity.ContentID) bool {
	if !summary.Available() || !mount.Available() || !artifact.Available() || !body.Available() {
		return false
	}
	operand, result, representationsOK := summary.Representations()
	return summary.ID() != summary.OccurrenceID() && summary.BodyPathID() == body && summary.OutputPointID().Available() &&
		flowkind.UnaryOp(summary.Operator()) == flowkind.UnaryNeg && representationsOK && operand.Valid() && result.Valid() && operand == result
}

// nativeNumericRepresentation is this publication's single spelling of the
// sealed representation vocabulary: the names exist nowhere else, so they are
// rendered here rather than projected from a declared row.
func nativeNumericRepresentation(representation cold.NumericRepresentation) (string, bool) {
	switch representation {
	case cold.NumericRepresentationInteger:
		return "integer", true
	case cold.NumericRepresentationFloat:
		return "float", true
	case cold.NumericRepresentationNumber:
		return "number", true
	default:
		return "", false
	}
}

// nativeArithmeticOperator is this publication's single spelling of the sealed
// arithmetic range of flowkind.BinaryOp; the operators carry no declared name
// of their own, and every member outside that range fails closed.
func nativeArithmeticOperator(op flowkind.BinaryOp) (string, bool) {
	switch op {
	case flowkind.BinaryAdd:
		return "add", true
	case flowkind.BinarySub:
		return "sub", true
	case flowkind.BinaryMul:
		return "mul", true
	case flowkind.BinaryDiv:
		return "div", true
	case flowkind.BinaryIDiv:
		return "idiv", true
	case flowkind.BinaryMod:
		return "mod", true
	case flowkind.BinaryPow:
		return "pow", true
	default:
		return "", false
	}
}

// nativeArithmeticDivisor is this publication's single spelling of the sealed
// divisor-property vocabulary; the guard conclusions carry no declared name of
// their own, and the absent property renders as no clause at all.
func nativeArithmeticDivisor(property cold.ArithmeticDivisorProperty) (string, bool) {
	switch property {
	case cold.ArithmeticDivisorNone:
		return "", true
	case cold.ArithmeticDivisorNonzero:
		return "nonzero", true
	case cold.ArithmeticDivisorNonzeroNotMinusOne:
		return "nonzero_not_minus_one", true
	default:
		return "", false
	}
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
	summary cold.ExactScalarSummary,
	mount, artifact, body, point identity.ContentID,
) bool {
	literal, ok := nativeSummaryLiteral(summary, mount, artifact, body, point)
	if !ok {
		return false
	}
	representation, rendered, ok := renderNativeLiteral(literal)
	if !ok {
		return false
	}
	if !appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyConstantValue, summary, mount, artifact, body, point, "representation="+representation+" value="+rendered) {
		return false
	}
	return appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyRepresentation, summary, mount, artifact, body, point, "exact=true representation="+representation)
}

func appendNativeStaticScalarRow(
	rows *[]nativePublicationRow,
	seen map[identity.ContentID]struct{},
	family nativePublicationFamily,
	summary cold.ExactScalarSummary,
	mount, artifact, body, point identity.ContentID,
	value string,
) bool {
	semantic, semanticOK := family.semanticID()
	if _, ok := nativeSummaryLiteral(summary, mount, artifact, body, point); !ok || !semanticOK || value == "" {
		return false
	}
	proof, occurrence, span := summary.ID(), summary.OccurrenceID(), summary.SubjectID()
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + proof.String() + "/" + point.String(), module: mount.String(),
		term: proof.String(), subject: span.String(), occurrence: occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: mount, artifact: artifact, local: occurrence, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
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

func nativeSummaryLiteral(summary cold.ExactScalarSummary, mount, artifact, body, point identity.ContentID) (keyspace.LiteralValue, bool) {
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

func validNativeValueSummary(observation valuedomain.ValueSummaryObservation, width int) bool {
	return observation.Valid && observation.Rows <= 1 && len(observation.Values) == width && len(observation.Present) == width
}

func appendNativeScalarRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, schema *valuedomain.Schema, value valuedomain.Value, coordinate identity.ContentID, subject anadiag.Observation, body, point identity.ContentID) bool {
	scalar, exact := schema.ExactScalar(value)
	if !exact {
		return true
	}
	representation, rendered, ok := renderNativeExactScalar(scalar)
	if !ok {
		return true
	}
	constant := "representation=" + representation + " value=" + rendered
	if !appendNativeBranchRow(rows, seen, nativePublicationFamilyConstantValue, subject, body, point, coordinate, constant) {
		return false
	}
	return appendNativeBranchRow(rows, seen, nativePublicationFamilyRepresentation, subject, body, point, coordinate, "exact=true representation="+representation)
}

func appendNativeBranchRows(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, subject anadiag.Observation, body identity.ContentID, truth valuedomain.Truth, complete bool) bool {
	classification := "dynamic_nil_or_false"
	partition := "partition=dynamic"
	if complete {
		switch truth {
		case valuedomain.TruthTrue:
			classification = "always_truthy"
			partition = "partition=always_taken dead_arm=else dead_arm_reachable=false"
		case valuedomain.TruthFalse:
			classification = "always_falsy"
			partition = "partition=always_not_taken dead_arm=then dead_arm_reachable=false"
		}
	}
	point := subject.Points[0]
	coordinate := identity.ContentID{}
	// Branch rows are keyed by the exact Program-issued observation identity,
	// not by a caller-provided coordinate or rendered source string.
	coordinate = subject.ID
	if !appendNativeBranchRow(rows, seen, nativePublicationFamilyTruthinessClass, subject, body, point, coordinate, "class="+classification) {
		return false
	}
	return appendNativeBranchRow(rows, seen, nativePublicationFamilyBranchPartition, subject, body, point, coordinate, partition)
}

func appendNativeBranchRow(rows *[]nativePublicationRow, seen map[identity.ContentID]struct{}, family nativePublicationFamily, subject anadiag.Observation, body, point, coordinate identity.ContentID, value string) bool {
	semantic, semanticOK := family.semanticID()
	span, spanOK := nativePublicationSpanID(subject.Location)
	if !semanticOK || !spanOK || !coordinate.Available() || !body.Available() || !point.Available() || value == "" {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + coordinate.String() + "/" + point.String(), module: subject.Location.File,
		term: coordinate.String(), subject: coordinate.String(), occurrence: subject.ID.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: subject.Mount, artifact: subject.Artifact, local: coordinate, body: body, point: point, span: span},
		provenanceOK: true, validityOK: true,
	}
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

// renderNativeLiteral is the sole spelling of a published Lua scalar. Both the
// exact-scalar path and the reusable-summary path consult it, so a constant
// reads the same however it was proved.
func renderNativeLiteral(literal keyspace.LiteralValue) (representation, value string, ok bool) {
	switch literal.Kind {
	case keyspace.LiteralBool:
		return "boolean", strconv.FormatBool(literal.Bool), true
	case keyspace.LiteralInteger:
		return "integer", strconv.FormatInt(literal.Integer, 10), true
	case keyspace.LiteralFloat:
		float := math.Float64frombits(literal.FloatBits)
		if math.IsNaN(float) || math.IsInf(float, 0) {
			return "", "", false
		}
		rendered := strconv.FormatFloat(float, 'g', -1, 64)
		if !strings.ContainsAny(rendered, ".eE") {
			rendered += ".0"
		}
		return "float", rendered, true
	case keyspace.LiteralString:
		return "string", strconv.Quote(literal.String), true
	default:
		return "", "", false
	}
}

// renderNativeNil is Lua nil's rendering. It stands outside the literal table
// because nil retains its own identity and has no keyspace literal to render.
func renderNativeNil() (representation, value string, ok bool) {
	return "nil", "nil", true
}

func renderNativeExactScalar(scalar valuedomain.ExactScalar) (representation, value string, ok bool) {
	switch scalar.Kind() {
	case valuedomain.ExactScalarNil:
		return renderNativeNil()
	case valuedomain.ExactScalarBoolean, valuedomain.ExactScalarLiteral:
		literal, literalOK := scalar.Literal()
		if !literalOK {
			return "", "", false
		}
		return renderNativeLiteral(literal)
	default:
		return "", "", false
	}
}
