package analysis

import (
	"encoding/binary"
	"math"
	"strconv"
	"strings"

	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	"github.com/wippyai/go-lua/analysis/engine"
	"github.com/wippyai/go-lua/analysis/internal/programartifact"
	flowkind "github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	programsource "github.com/wippyai/go-lua/program/source"
)

// buildNativeBranchPublication is the sole post-convergence owner for the
// initial native Value families. It consumes only Program-issued mounted
// branch geometry and Engine-authenticated Value observations. Manifest rows
// and diagnostic policy never enter this producer.
func buildNativeBranchPublication(
	receipt *artifactResultReceipt,
	selected []artifactDiagnosticObservationReceipt,
	schema *valuedomain.Schema,
	solver *engine.Solver,
	state *engine.State,
) (*nativePublicationReceipt, bool) {
	if !receipt.valid() || schema == nil || solver == nil || state == nil {
		return nil, false
	}
	observed := make(map[artifactResultPoint]valueSummaryObservation, len(selected))
	for _, selectedObservation := range selected {
		if !selectedObservation.point.mount.Available() || !selectedObservation.point.point.Available() {
			return nil, false
		}
		if _, duplicate := observed[selectedObservation.point]; duplicate {
			return nil, false
		}
		observation, readable := engine.ReceiptObservationResult(selectedObservation.observation, solver, state)
		if !readable || !validNativeValueSummary(observation, len(receipt.values)) {
			return nil, false
		}
		observed[selectedObservation.point] = observation
	}
	rows := make([]nativePublicationRow, 0)
	byID := make(map[keyspace.ContentID]struct{})
	for _, source := range receipt.nativeScalars {
		if !appendNativeStaticScalarRows(&rows, byID, source) {
			return nil, false
		}
	}
	for _, summary := range receipt.nativeArithmetics {
		if !appendNativeArithmeticRows(&rows, byID, summary) {
			return nil, false
		}
	}
	for _, summary := range receipt.nativeUnaries {
		if !appendNativeUnaryRows(&rows, byID, summary) {
			return nil, false
		}
	}
	expected := make(map[artifactResultPoint]struct{})
	for _, subject := range receipt.branchObservations {
		if subject.kind != programartifact.DiagnosticObservationBranchCondition || !subject.available() || int(subject.valueIndex) >= len(receipt.values) {
			return nil, false
		}
		truth := valuedomain.TruthNone
		complete := true
		var subjectBody keyspace.ContentID
		for _, point := range subject.points {
			key := artifactResultPoint{mount: subject.mount, point: point}
			expected[key] = struct{}{}
			body, bodyOK := nativePublicationBodyAt(receipt, key)
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
			condition := int(subject.valueIndex)
			if !observation.Present[condition] {
				complete = false
				continue
			}
			// A branch observation authenticates the condition coordinate. Other
			// cells share its point snapshot but are not native-publication uses;
			// publishing them globally leaks path-local literals across merges.
			if !appendNativeScalarRows(&rows, byID, schema, observation.Values[condition], receipt.values[condition], subject, subjectBody, point) {
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

func appendNativeArithmeticRows(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, summary compiledNativeArithmeticSummary) bool {
	left, leftOK := nativeNumericRepresentation(summary.left)
	right, rightOK := nativeNumericRepresentation(summary.right)
	result, resultOK := nativeNumericRepresentation(summary.result)
	operator, operatorOK := nativeArithmeticOperator(summary.op)
	overflow, overflowOK := nativeArithmeticOverflow(summary.op, summary.left, summary.right)
	divisor, divisorOK := nativeArithmeticDivisor(summary.divisor)
	if !summary.valid() || !leftOK || !rightOK || !resultOK || !operatorOK || !overflowOK || !divisorOK {
		return false
	}
	representation := "representation=" + result + " left=" + left + " operator=" + operator + " overflow=" + overflow + " result_representation=" + result + " right=" + right
	if summary.result != programartifact.NumericRepresentationNumber {
		representation = "exact=true " + representation
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyRepresentation, summary, representation) {
		return false
	}
	operatorValue := "class=number dispatch=primitive left=" + left + " operator=" + operator + " overflow=" + overflow + " result=" + result + " right=" + right
	if divisor != "" {
		operatorValue += " divisor=" + divisor
	}
	if !appendNativeArithmeticRow(rows, seen, nativePublicationFamilyScalarOperator, summary, operatorValue) {
		return false
	}
	if summary.op == flowkind.BinaryDiv {
		return appendNativeArithmeticRow(rows, seen, nativePublicationFamilyDivisorProperty, summary, "divisor=not_applicable operator=div")
	}
	if divisor != "" {
		return appendNativeArithmeticRow(rows, seen, nativePublicationFamilyDivisorProperty, summary, "divisor="+divisor+" operator="+operator)
	}
	return true
}

func appendNativeArithmeticRow(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, family nativePublicationFamily, summary compiledNativeArithmeticSummary, value string) bool {
	semantic, semanticOK := family.semanticID()
	if !summary.valid() || !semanticOK || value == "" {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + summary.proof.String() + "/" + summary.point.String(), module: summary.mount.String(),
		term: summary.proof.String(), subject: summary.span.String(), occurrence: summary.occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: summary.mount, artifact: summary.artifact, local: summary.occurrence, body: summary.body, point: summary.point, span: summary.span},
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

func appendNativeUnaryRows(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, summary compiledNativeUnarySummary) bool {
	operand, operandOK := nativeNumericRepresentation(summary.operand)
	result, resultOK := nativeNumericRepresentation(summary.result)
	if !summary.valid() || !operandOK || !resultOK {
		return false
	}
	overflow := "ieee754"
	if summary.operand == programartifact.NumericRepresentationInteger {
		overflow = "closed_integer"
	}
	value := "operator=unm overflow=" + overflow + " representation=" + result + " result_representation=" + result + " operand_representation=" + operand
	if summary.result != programartifact.NumericRepresentationNumber {
		value = "exact=true " + value
	}
	semantic, semanticOK := nativePublicationFamilyRepresentation.semanticID()
	if !semanticOK {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: nativePublicationFamilyRepresentation, trust: NativePublicationTrustProven,
		key: nativePublicationFamilyRepresentation.String() + "/" + summary.proof.String() + "/" + summary.point.String(), module: summary.mount.String(),
		term: summary.proof.String(), subject: summary.span.String(), occurrence: summary.occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: summary.mount, artifact: summary.artifact, local: summary.occurrence, body: summary.body, point: summary.point, span: summary.span},
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

func nativeNumericRepresentation(representation programartifact.NumericRepresentation) (string, bool) {
	switch representation {
	case programartifact.NumericRepresentationInteger:
		return "integer", true
	case programartifact.NumericRepresentationFloat:
		return "float", true
	case programartifact.NumericRepresentationNumber:
		return "number", true
	default:
		return "", false
	}
}

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

func nativeArithmeticOverflow(op flowkind.BinaryOp, left, right programartifact.NumericRepresentation) (string, bool) {
	if !left.Valid() || !right.Valid() {
		return "", false
	}
	switch op {
	case flowkind.BinaryAdd, flowkind.BinarySub, flowkind.BinaryMul:
		if left == programartifact.NumericRepresentationInteger && right == programartifact.NumericRepresentationInteger {
			return "promote_integer_to_number", true
		}
		return "ieee754", true
	case flowkind.BinaryIDiv, flowkind.BinaryMod:
		if left == programartifact.NumericRepresentationInteger && right == programartifact.NumericRepresentationInteger {
			return "closed_integer", true
		}
		return "ieee754", true
	case flowkind.BinaryDiv, flowkind.BinaryPow:
		return "ieee754", true
	default:
		return "", false
	}
}

func nativeArithmeticDivisor(property programartifact.ArithmeticDivisorProperty) (string, bool) {
	switch property {
	case programartifact.ArithmeticDivisorNone:
		return "", true
	case programartifact.ArithmeticDivisorNonzero:
		return "nonzero", true
	case programartifact.ArithmeticDivisorNonzeroNotMinusOne:
		return "nonzero_not_minus_one", true
	default:
		return "", false
	}
}

func nativePublicationBodyAt(receipt *artifactResultReceipt, point artifactResultPoint) (keyspace.ContentID, bool) {
	if receipt == nil {
		return keyspace.ContentID{}, false
	}
	indexes := receipt.pointBodies[point]
	if len(indexes) != 1 || indexes[0] < 0 || indexes[0] >= len(receipt.bodies) {
		return keyspace.ContentID{}, false
	}
	body := receipt.bodies[indexes[0]].key.body
	return body, body.Available()
}

func appendNativeStaticScalarRows(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, source compiledNativeScalarSource) bool {
	representation, rendered, ok := renderNativeScalarSummary(source)
	if !ok {
		return false
	}
	if !appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyConstantValue, source, "representation="+representation+" value="+rendered) {
		return false
	}
	return appendNativeStaticScalarRow(rows, seen, nativePublicationFamilyRepresentation, source, "exact=true representation="+representation)
}

func appendNativeStaticScalarRow(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, family nativePublicationFamily, source compiledNativeScalarSource, value string) bool {
	semantic, semanticOK := family.semanticID()
	if !source.valid() || !semanticOK || value == "" {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + source.proof.String() + "/" + source.point.String(), module: source.mount.String(),
		term: source.proof.String(), subject: source.span.String(), occurrence: source.occurrence.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: source.mount, artifact: source.artifact, local: source.occurrence, body: source.body, point: source.point, span: source.span},
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

func validNativeValueSummary(observation valueSummaryObservation, width int) bool {
	return observation.Valid && observation.Rows <= 1 && len(observation.Values) == width && len(observation.Present) == width
}

func appendNativeScalarRows(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, schema *valuedomain.Schema, value valuedomain.Value, coordinate keyspace.ContentID, subject compiledObservation, body, point keyspace.ContentID) bool {
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

func appendNativeBranchRows(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, subject compiledObservation, body keyspace.ContentID, truth valuedomain.Truth, complete bool) bool {
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
	point := subject.points[0]
	coordinate := keyspace.ContentID{}
	// Branch rows are keyed by the exact Program-issued observation identity,
	// not by a caller-provided coordinate or rendered source string.
	coordinate = subject.id
	if !appendNativeBranchRow(rows, seen, nativePublicationFamilyTruthinessClass, subject, body, point, coordinate, "class="+classification) {
		return false
	}
	return appendNativeBranchRow(rows, seen, nativePublicationFamilyBranchPartition, subject, body, point, coordinate, partition)
}

func appendNativeBranchRow(rows *[]nativePublicationRow, seen map[keyspace.ContentID]struct{}, family nativePublicationFamily, subject compiledObservation, body, point, coordinate keyspace.ContentID, value string) bool {
	semantic, semanticOK := family.semanticID()
	span, spanOK := nativePublicationSpanID(subject.location)
	if !semanticOK || !spanOK || !coordinate.Available() || !body.Available() || !point.Available() || value == "" {
		return false
	}
	row := nativePublicationRow{
		semantic: semantic,
		lane:     NativePublicationLaneValues, kind: NativePublicationKindValue, family: family, trust: NativePublicationTrustProven,
		key: family.String() + "/" + coordinate.String() + "/" + point.String(), module: subject.location.File,
		term: coordinate.String(), subject: coordinate.String(), occurrence: subject.id.String(), value: value, valueOK: true,
		provenance:   NativePublicationProvenance{mount: subject.mount, artifact: subject.artifact, local: coordinate, body: body, point: point, span: span},
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

func nativePublicationSpanID(span programsource.Span) (keyspace.ContentID, bool) {
	if !validMountedDiagnosticSpan(span) {
		return keyspace.ContentID{}, false
	}
	var coordinates [16]byte
	binary.BigEndian.PutUint32(coordinates[0:4], span.StartLine)
	binary.BigEndian.PutUint32(coordinates[4:8], span.StartCol)
	binary.BigEndian.PutUint32(coordinates[8:12], span.EndLine)
	binary.BigEndian.PutUint32(coordinates[12:16], span.EndCol)
	return analysisContentID("analysis/native-publication/source-span/v1", []byte(span.File), coordinates[:])
}

func renderNativeExactScalar(scalar valuedomain.ExactScalar) (representation, value string, ok bool) {
	switch scalar.Kind() {
	case valuedomain.ExactScalarNil:
		return "nil", "nil", true
	case valuedomain.ExactScalarBoolean, valuedomain.ExactScalarLiteral:
		literal, literalOK := scalar.Literal()
		if !literalOK {
			return "", "", false
		}
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
	default:
		return "", "", false
	}
}

func renderNativeScalarSummary(source compiledNativeScalarSource) (representation, value string, ok bool) {
	if !source.valid() {
		return "", "", false
	}
	if source.family == keyspace.FamilyNil {
		return "nil", "nil", true
	}
	literal := source.literal
	switch source.family {
	case keyspace.FamilyBool:
		return "boolean", strconv.FormatBool(literal.Bool), true
	case keyspace.FamilyInteger:
		return "integer", strconv.FormatInt(literal.Integer, 10), true
	case keyspace.FamilyFloat:
		float := math.Float64frombits(literal.FloatBits)
		if math.IsNaN(float) || math.IsInf(float, 0) {
			return "", "", false
		}
		rendered := strconv.FormatFloat(float, 'g', -1, 64)
		if !strings.ContainsAny(rendered, ".eE") {
			rendered += ".0"
		}
		return "float", rendered, true
	case keyspace.FamilyString:
		return "string", strconv.Quote(literal.String), true
	default:
		return "", "", false
	}
}
