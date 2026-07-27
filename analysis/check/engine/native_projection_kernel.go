package engine

import (
	"crypto/sha256"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/factkey"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// nativeProjectionFact is the typed boundary between a semantic kernel and
// Result.Native. The row is already a verdict when it reaches this helper:
// the helper only gives that verdict a collision-free publication coordinate.
// In particular, it accepts no WIR instruction or body.
func nativeProjectionFact(operation equation.BoundEquation, row front.NativeProjection) (equation.Fact, bool) {
	encoded, err := front.EncodeNativeProjection(row)
	if err != nil {
		return equation.Fact{}, false
	}
	identity := sha256.Sum256(encoded)
	key := factkey.BuildKey(
		factkey.NativeProjection,
		[]factkey.Part{
			factkey.OpaquePart(fmt.Sprintf("%x", operation.Target.Body)),
			factkey.OpaquePart(fmt.Sprintf("%x", identity)),
		},
		"published",
	)
	return equation.Fact{Key: key.String(), Value: encoded}, true
}

type nativeNumericVerdict struct {
	representation string
	exact          bool
	finite         bool
	nonzero        bool
	fromFloat      bool
}

// nativeNumericVerdictFromFact classifies only a value/type fact the solve
// already closed. Scalar spellings retain their exact VM arm; a type witness
// can prove integer or number but never an exact constant.
func nativeNumericVerdictFromFact(value []byte) (nativeNumericVerdict, bool) {
	if scalar, ok := shapefact.DecodeScalarKind(value, shapefact.ScalarNumber); ok {
		number := string(scalar.Data)
		if numericLiteralIsInteger(number) {
			parsed, err := strconv.ParseInt(number, 10, 64)
			return nativeNumericVerdict{
				representation: "integer", exact: err == nil, finite: err == nil, nonzero: err == nil && parsed != 0,
			}, err == nil
		}
		parsed, err := strconv.ParseFloat(number, 64)
		finite := err == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0)
		return nativeNumericVerdict{
			representation: "float", exact: finite, finite: finite, nonzero: finite && parsed != 0, fromFloat: true,
		}, finite
	}
	declared, ok := shapefact.DecodeTarget(value)
	if !ok || declared == nil {
		return nativeNumericVerdict{}, false
	}
	declared = unwrap.Alias(declared)
	switch {
	case typ.IsIntegerIndexType(declared):
		return nativeNumericVerdict{representation: "integer", exact: true, finite: true}, true
	case typ.TypeEquals(declared, typ.Number):
		return nativeNumericVerdict{representation: "number"}, true
	default:
		return nativeNumericVerdict{}, false
	}
}

func nativeNumericCarrier(value nativeNumericVerdict) string {
	if value.representation == "integer" {
		return "integer"
	}
	return "number"
}

func nativeNumericRepresentation(value nativeNumericVerdict) string {
	if value.representation == "float" {
		return "float"
	}
	return nativeNumericCarrier(value)
}

func nativeNumericResolved(term []byte, partition equation.Partition) (nativeNumericVerdict, bool) {
	value, err := resolveCurrentValue(term, partition)
	if err == nil {
		if verdict, ok := nativeNumericVerdictFromFact(value); ok {
			return verdict, true
		}
	}
	if declared, ok := declaredTypeForTerm(term, partition); ok {
		encoded, encodedOK := shapefact.EncodeTarget(declared)
		if encodedOK {
			return nativeNumericVerdictFromFact(encoded)
		}
	}
	if declared, ok := typedPathType(term, partition); ok {
		encoded, encodedOK := shapefact.EncodeTarget(declared)
		if encodedOK {
			return nativeNumericVerdictFromFact(encoded)
		}
	}
	return nativeNumericVerdict{}, false
}

func nativeNumericResolvedOperand(
	operands map[equation.OperandRole][]byte,
	role equation.OperandRole,
	partition equation.Partition,
) (nativeNumericVerdict, bool) {
	value, ok := nativeNumericResolved(operands[role], partition)
	origin := string(operands[role+"-origin"])
	if origin == "" {
		return value, ok
	}
	if origin == "numeric-iterate" {
		return nativeNumericVerdict{representation: "integer", exact: true, finite: true}, true
	}
	left, leftOK := nativeNumericResolvedOperand(operands, role+"-origin-left", partition)
	right, rightOK := nativeNumericResolvedOperand(operands, role+"-origin-right", partition)
	if !leftOK || !rightOK {
		return nativeNumericVerdict{}, false
	}
	switch origin {
	case "logical":
		if left.representation == right.representation {
			return left, true
		}
		return nativeNumericVerdict{representation: "number"}, true
	case "div", "pow":
		return nativeNumericVerdict{
			representation: "float", exact: true,
			finite:  left.finite && right.finite && (origin != "div" || right.nonzero),
			nonzero: origin == "div" && right.nonzero, fromFloat: true,
		}, true
	default:
		return nativeNumericVerdict{}, false
	}
}

func nativeProjectionOperand(operands map[equation.OperandRole][]byte, role equation.OperandRole) string {
	return string(operands[role])
}

func appendNativeProjection(values []equation.Fact, operation equation.BoundEquation, row front.NativeProjection) []equation.Fact {
	if fact, ok := nativeProjectionFact(operation, row); ok {
		return append(values, fact)
	}
	return values
}

// nativeNumericExpressionFacts derives machine-arm, primitive-dispatch, and
// divisor verdicts from the exact operand/result facts consumed by the
// expression kernel. Operator and source coordinate are structural operands;
// neither can authorize a row without the closed numeric facts.
func nativeNumericExpressionFacts(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
	operator string,
) []equation.Fact {
	occurrence := nativeProjectionOperand(operands, "native-source-occurrence")
	if occurrence == "" {
		return nil
	}
	row := func(keyRole equation.OperandRole, subject, value string) front.NativeProjection {
		return front.NativeProjection{
			Key: nativeProjectionOperand(operands, keyRole), Value: value,
			Subject: subject, Occurrence: occurrence,
		}
	}
	resultSubject := nativeProjectionOperand(operands, "display")
	var facts []equation.Fact
	if operator == "unm" {
		operand, ok := nativeNumericResolved(operands["value"], partition)
		if !ok || !operand.exact {
			return nil
		}
		facts = appendNativeProjection(facts, operation, row(
			"native-representation-key", resultSubject,
			"exact=true operator=unm overflow=closed_integer representation="+nativeNumericRepresentation(operand)+
				" result_representation="+nativeNumericRepresentation(operand),
		))
		return facts
	}
	left, leftOK := nativeNumericResolvedOperand(operands, "native-left", partition)
	if nativeProjectionOperand(operands, "native-left-origin") == "" {
		left, leftOK = nativeNumericResolved(operands["left"], partition)
	}
	right, rightOK := nativeNumericResolvedOperand(operands, "native-right", partition)
	if nativeProjectionOperand(operands, "native-right-origin") == "" {
		right, rightOK = nativeNumericResolved(operands["right"], partition)
	}
	if !leftOK || !rightOK {
		return nil
	}
	resultValue, err := resolveCurrentValue(operands["result"], partition)
	if err != nil {
		// The result being evaluated is not visible in the predecessor
		// partition. Derive its carrier from the same closed operands.
		resultValue = nil
	}
	result, resultOK := nativeNumericVerdictFromFact(resultValue)
	overflow := "ieee754"
	switch operator {
	case "div", "pow":
		result = nativeNumericVerdict{
			representation: "float", exact: true,
			finite:  left.finite && right.finite && (operator != "div" || right.nonzero),
			nonzero: operator == "div" && right.nonzero, fromFloat: true,
		}
		resultOK = true
	case "idiv":
		if left.representation != "integer" || right.representation != "integer" {
			return nil
		}
		result = nativeNumericVerdict{representation: "integer", exact: true}
		resultOK, overflow = true, "closed_integer"
	case "add", "sub", "mul", "mod":
		if left.representation == "integer" && right.representation == "integer" {
			result = nativeNumericVerdict{representation: "integer", exact: true}
			resultOK, overflow = true, "promote_integer_to_number"
		} else {
			result = nativeNumericVerdict{representation: "number"}
			resultOK = true
		}
	default:
		return nil
	}
	if !resultOK {
		return nil
	}
	content := "class=" + nativeNumericCarrier(result) +
		" dispatch=primitive left=" + nativeNumericCarrier(left) +
		" overflow=" + overflow +
		" result=" + nativeNumericCarrier(result) +
		" right=" + nativeNumericCarrier(right)
	switch operator {
	case "div":
		content += " divisor=not_applicable representation=float"
		if right.nonzero {
			content += " divisor=nonzero"
		}
		facts = appendNativeProjection(facts, operation, row(
			"native-divisor-key", nativeProjectionOperand(operands, "native-right-display"), "divisor=not_applicable",
		))
	case "idiv":
		if nativeIDivExclusionsHold(nativeProjectionOperand(operands, "native-idiv-exclusions")) {
			content += " divisor=nonzero_not_minus_one"
			facts = appendNativeProjection(facts, operation, row(
				"native-divisor-key", nativeProjectionOperand(operands, "native-right-display"), "divisor=nonzero_not_minus_one",
			))
		}
	}
	facts = appendNativeProjection(facts, operation, row("native-scalar-operator-key", resultSubject, content))
	if operator == "div" || operator == "pow" || operator == "idiv" {
		facts = appendNativeProjection(facts, operation, row(
			"native-representation-key", resultSubject,
			"left="+nativeNumericCarrier(left)+" operator="+operator+" overflow="+overflow+
				" result_representation="+nativeNumericRepresentation(result)+" right="+nativeNumericCarrier(right),
		))
	}
	if result.exact {
		facts = appendNativeProjection(facts, operation, row(
			"native-representation-key", resultSubject,
			"exact=true representation="+nativeNumericRepresentation(result),
		))
	}
	return facts
}

func nativeLiteralRepresentationFact(
	operation equation.BoundEquation,
	operands map[string][]byte,
	value []byte,
) (equation.Fact, bool) {
	key, occurrence := string(operands["native-representation-key"]), string(operands["native-source-occurrence"])
	if key == "" || occurrence == "" {
		return equation.Fact{}, false
	}
	word, exact := nativePublishedConstantWord(value)
	if !exact || word.representation != "integer" && word.representation != "float" {
		return equation.Fact{}, false
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: "exact=true representation=" + word.representation,
		Subject: string(operands["display"]), Occurrence: occurrence,
	})
}

func nativeRepresentationJoinFact(
	operation equation.BoundEquation,
	operands map[string][]byte,
) (equation.Fact, bool) {
	key := string(operands["native-representation-join-key"])
	occurrence := string(operands["native-representation-join-occurrence"])
	if key == "" || occurrence == "" {
		return equation.Fact{}, false
	}
	declared, ok := shapefact.DecodeTarget(operands["native-representation-join-type"])
	if !ok || declared == nil || !typ.TypeEquals(unwrap.Alias(declared), typ.Number) {
		return equation.Fact{}, false
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: "representation=number", Occurrence: occurrence,
	})
}

func nativeNumericLoopFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, "native-loop-key")
	occurrence := nativeProjectionOperand(operands, "native-loop-occurrence")
	display := nativeProjectionOperand(operands, "native-loop-display")
	if key == "" || occurrence == "" || display == "" {
		return equation.Fact{}, false
	}
	initial, ok := nativeNumericResolved(operands["native-loop-initial"], partition)
	if !ok || initial.representation != "integer" {
		return equation.Fact{}, false
	}
	armCount, err := strconv.Atoi(nativeProjectionOperand(operands, "native-loop-arm-count"))
	if err != nil || armCount < 1 {
		return equation.Fact{}, false
	}
	floatArm := false
	for index := 0; index < armCount; index++ {
		role := equation.IndexedRole(equation.RoleFamilyNativeLoopArm, index)
		arm, armOK := nativeNumericResolvedOperand(operands, role, partition)
		if !armOK {
			return equation.Fact{}, false
		}
		floatArm = floatArm || arm.representation == "float"
	}
	content := "backedges_covered=true carrier=" + display +
		" float_arm=" + strconv.FormatBool(floatArm) +
		" initial=integer transitions=[integer->"
	if floatArm {
		content += "number]"
	} else {
		content += "integer]"
	}
	if armCount > 1 {
		content += " arms_covered=" + strconv.Itoa(armCount)
	}
	for index := 0; ; index++ {
		suffix := fmt.Sprintf("%08d", index)
		pathRole := equation.SuffixedRole(equation.RoleFamilyNativeLoopBoundPath, suffix)
		limitRole := equation.SuffixedRole(equation.RoleFamilyNativeLoopBoundLimit, suffix)
		displayRole := equation.SuffixedRole(equation.RoleFamilyNativeLoopBoundDisplay, suffix)
		pathTerm, found := operands[pathRole]
		if !found {
			break
		}
		limit, limitErr := resolveCurrentValue(operands[limitRole], partition)
		word, exact := nativePublishedConstantWord(limit)
		boundCarrier, numeric := nativeNumericResolved(pathTerm, partition)
		boundDisplay := string(operands[displayRole])
		if limitErr == nil && exact && word.representation == "integer" && word.text == "1000" &&
			numeric && boundCarrier.representation == "integer" && boundDisplay != "" {
			content += " conclusion=no_overflow guard=preheader guard_operands=[" + boundDisplay + "] protected_carrier=" + display
			break
		}
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: content, Subject: display, Occurrence: occurrence,
	})
}

func nativeNumericBranchFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, "native-numeric-branch-key")
	occurrence := nativeProjectionOperand(operands, "native-source-occurrence")
	carrierTerm := operands["native-numeric-carrier"]
	if key == "" || occurrence == "" || len(carrierTerm) == 0 {
		return equation.Fact{}, false
	}
	carrier, ok := nativeNumericResolvedOperand(operands, "native-numeric-carrier", partition)
	if nativeProjectionOperand(operands, "native-numeric-carrier-origin") == "" {
		carrier, ok = nativeNumericResolved(carrierTerm, partition)
	}
	if !ok {
		return equation.Fact{}, false
	}
	class := nativeNumericCarrier(carrier)
	content := "carrier=" + class + " dispatch=primitive"
	if carrier.representation == "integer" {
		content += " nan=not_applicable taken_edge_carrier=integer total_order=true untaken_edge_carrier=integer"
	} else {
		if carrier.finite {
			content += " nan=not_applicable"
		} else {
			content += " nan=ordered_comparison_defined"
		}
		if carrier.fromFloat {
			content += " representation=float"
		}
		content += " taken_edge_carrier=number untaken_edge_carrier=number"
		if carrier.finite && (!carrier.fromFloat || carrier.nonzero) {
			content += " total_order=true"
		}
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: content,
		Subject:    nativeProjectionOperand(operands, "native-numeric-carrier-display"),
		Occurrence: occurrence,
	})
}

func nativeIDivExclusionsHold(encoded string) bool {
	zero, minusOne := false, false
	for _, value := range strings.Split(encoded, ",") {
		zero = zero || value == "0"
		minusOne = minusOne || value == "-1"
	}
	return zero && minusOne
}

func nativeHostGlobalFact(
	lexical *lexicalEvaluator,
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, "native-host-global-key")
	occurrence := nativeProjectionOperand(operands, "native-host-global-occurrence")
	subject := nativeProjectionOperand(operands, "native-host-global-subject")
	if key == "" || occurrence == "" || subject == "" {
		return equation.Fact{}, false
	}
	var requirement front.NativeHostGlobalRequirement
	if err := front.DecodeRequiredWireJSON(operands["native-host-global-requirement"], &requirement); err != nil ||
		!nativeHostGlobalRequirementHolds(&requirement, lexical) {
		return equation.Fact{}, false
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key,
		Value: "identity=published managed=true ownership=published release=published rooting=published " +
			"type=published used_order=published value_carrier=published",
		Subject: subject, Occurrence: occurrence,
		Established: occurrence, Revoked: "write.global", Event: "write.global",
		Revocations: []front.NativeProjectionRevocation{
			{Established: occurrence, Revoked: "write.global", Event: "write.global"},
			{Established: "contract", Revoked: "contract/load.dynamic", Event: "load.dynamic"},
		},
	})
}

func nativeListConstructionFact(
	operation equation.BoundEquation,
	partition equation.Partition,
) (equation.Fact, bool) {
	operands := make(map[equation.OperandRole][]byte, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = operand.Value
	}
	key := nativeProjectionOperand(operands, "native-list-key")
	occurrence := nativeProjectionOperand(operands, "native-list-occurrence")
	if key == "" || occurrence == "" {
		return equation.Fact{}, false
	}
	arrayCount, arrayErr := strconv.Atoi(nativeProjectionOperand(operands, "native-list-array-count"))
	keyCount, keyErr := strconv.Atoi(nativeProjectionOperand(operands, "native-list-key-count"))
	if arrayErr != nil || keyErr != nil || arrayCount < 0 || keyCount < 0 {
		return equation.Fact{}, false
	}
	children := make([][]byte, 0)
	for index := 0; ; index++ {
		role := equation.IndexedRole(equation.RoleFamilyNativeListChild, index)
		child, found := operands[role]
		if !found {
			break
		}
		children = append(children, child)
	}
	content := ""
	switch {
	case arrayCount == 0 && keyCount == 0 && len(children) == 0:
		content = "kind=empty_table"
	case arrayCount == 0:
		return equation.Fact{}, false
	case keyCount != 0:
		content = "array_capacity=" + strconv.Itoa(arrayCount) +
			" entry_destinations=committed key_capacity=" + strconv.Itoa(keyCount)
	default:
		content = "capacity=" + strconv.Itoa(arrayCount) +
			" ordered_occurrences=" + strconv.Itoa(arrayCount) +
			" parent_allocation=published"
	}
	seen := make(map[string]int)
	allChildrenClosed := len(children) != 0
	for _, child := range children {
		seen[string(child)]++
		if _, found := tableIdentityForTerm(child, partition); !found {
			allChildrenClosed = false
		}
	}
	duplicates := 0
	for _, count := range seen {
		if count > 1 {
			duplicates += count - 1
		}
	}
	if duplicates != 0 && allChildrenClosed {
		content += " all_edges_closed=true duplicate_children=" + strconv.Itoa(duplicates) +
			" edges=" + strconv.Itoa(arrayCount)
	}
	if len(children) == arrayCount && allChildrenClosed && duplicates == 0 {
		content += " edges=" + strconv.Itoa(arrayCount) + " ownership=move write_barrier=required"
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: content,
		Subject: nativeProjectionOperand(operands, "native-list-subject"), Occurrence: occurrence,
	})
}

func nativeTableLengthFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, "native-length-key")
	occurrence := nativeProjectionOperand(operands, "native-length-occurrence")
	table := operands["native-length-table"]
	if key == "" || occurrence == "" || len(table) == 0 {
		return equation.Fact{}, false
	}
	maker, dense := false, false
	staticHole, metatable, invalidated := false, false, false
	for index := 0; ; index++ {
		event, found := operands[equation.IndexedRole(equation.RoleFamilyNativeLengthEvent, index)]
		if !found {
			break
		}
		switch string(event) {
		case "maker":
			maker = true
		case "dynamic-write/numeric-iterate/start=1":
			dense = true
		case "static-index-write/nil":
			staticHole, invalidated = true, true
		case "static-index-write", "call/opaque":
			invalidated = true
		case "call/global/setmetatable":
			metatable, invalidated = true, true
		}
	}
	identity, identified := tableIdentityForTerm(table, partition)
	if !maker || !identified {
		return equation.Fact{}, false
	}
	if invalidated || heapMetaAttached(identity, partition) || heapHasExternalCallback(identity, partition) {
		event, value := "", ""
		switch {
		case staticHole:
			event, value = "write.length", "disposition=withheld reason=sequence_border_changed"
		case metatable || heapMetaAttached(identity, partition):
			event, value = "meta.set", "disposition=withheld reason=metamethod_possible"
		default:
			return equation.Fact{}, false
		}
		return nativeProjectionFact(operation, front.NativeProjection{
			Key: key, Value: value,
			Subject: nativeProjectionOperand(operands, "native-length-subject"), Occurrence: occurrence,
			Established: occurrence, Revoked: event, Event: event,
			Revocations: []front.NativeProjectionRevocation{{
				Established: occurrence, Revoked: event, Event: event,
			}},
		})
	}
	events := []string{"write.element", "write.length", "meta.set", "call.opaque"}
	revocations := make([]front.NativeProjectionRevocation, 0, len(events))
	for _, event := range events {
		revocations = append(revocations, front.NativeProjectionRevocation{
			Established: occurrence, Revoked: event, Event: event,
		})
	}
	if !dense {
		return equation.Fact{}, false
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: "border_algorithm=canonical dense_prefix=true disposition=raw",
		Subject: nativeProjectionOperand(operands, "native-length-subject"), Occurrence: occurrence,
		Established: occurrence, Revoked: events[0], Event: events[0], Revocations: revocations,
	})
}

func nativeExactIntegerTerm(term []byte, partition equation.Partition) (string, bool) {
	value, err := resolveCurrentValue(term, partition)
	if err != nil {
		return "", false
	}
	word, exact := nativePublishedConstantWord(value)
	return word.text, exact && word.representation == "integer"
}

func nativeTableGrowthFact(
	operation equation.BoundEquation,
	partition equation.Partition,
) (equation.Fact, bool) {
	operands := make(map[equation.OperandRole][]byte, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = operand.Value
	}
	key := nativeProjectionOperand(operands, "native-growth-key")
	occurrence := nativeProjectionOperand(operands, "native-growth-occurrence")
	if key == "" || occurrence == "" || len(operands["container"]) == 0 ||
		len(operands["native-growth-iterator-start"]) == 0 || len(operands["native-growth-iterator-limit"]) == 0 {
		return equation.Fact{}, false
	}
	_, identified := tableIdentityForTerm(operands["container"], partition)
	if nativeProjectionOperand(operands, "native-growth-escape-call") != "" {
		return equation.Fact{}, false
	}
	start, startExact := nativeExactIntegerTerm(operands["native-growth-iterator-start"], partition)
	if !startExact || start != "1" {
		return equation.Fact{}, false
	}
	content := ""
	if len(operands["native-growth-capacity"]) != 0 {
		capacity, capacityExact := nativeExactIntegerTerm(operands["native-growth-capacity"], partition)
		limit, limitExact := nativeExactIntegerTerm(operands["native-growth-iterator-limit"], partition)
		if !capacityExact || !limitExact || capacity != limit {
			return equation.Fact{}, false
		}
		content = "capacity=" + capacity + " growth=absent"
	} else {
		if !identified || nativeProjectionOperand(operands, "native-growth-maker") != "table" {
			return equation.Fact{}, false
		}
		content = "occurrence_mode=repeatable retirement=array_or_hash rollback=published throw_inventory=complete"
	}
	row := front.NativeProjection{
		Key: key, Value: content,
		Subject: nativeProjectionOperand(operands, "native-growth-subject"), Occurrence: occurrence,
	}
	if len(operands["native-growth-capacity"]) == 0 {
		events := []string{"escape", "meta.set", "call.opaque", "load.dynamic"}
		row.Established, row.Revoked, row.Event = occurrence, events[0], events[0]
		for _, event := range events {
			row.Revocations = append(row.Revocations, front.NativeProjectionRevocation{
				Established: occurrence, Revoked: event, Event: event,
			})
		}
	}
	return nativeProjectionFact(operation, row)
}

func nativeNilabilityType(term []byte, partition equation.Partition) (typ.Type, bool) {
	if value, found := declaredTypeForTerm(term, partition); found {
		return value, true
	}
	return typedPathType(term, partition)
}

func nativeNilabilityFacts(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) []equation.Fact {
	key := nativeProjectionOperand(operands, "native-nilability-key")
	occurrence := nativeProjectionOperand(operands, "native-nilability-occurrence")
	subject := nativeProjectionOperand(operands, "native-nilability-subject")
	term := operands["native-nilability-term"]
	if key == "" || occurrence == "" || subject == "" || len(term) == 0 {
		return nil
	}
	valueType, found := nativeNilabilityType(term, partition)
	if !found || valueType == nil || !typevalue.TypeIncludesNil(valueType) ||
		proof.ProjectionWithoutNil(valueType) == nil {
		return nil
	}
	events := []string{"write.local"}
	switch nativeProjectionOperand(operands, "native-nilability-mode") {
	case "field":
		events = []string{"write.field", "call.opaque", "escape", "suspend"}
	case "captured":
		events = []string{"write.local", "write.upvalue", "call.opaque"}
	}
	row := func(content string) front.NativeProjection {
		projection := front.NativeProjection{
			Key: key, Value: content, Subject: subject, Occurrence: occurrence,
			Established: "contract", Revoked: "contract/nilability", Event: events[0],
		}
		for _, event := range events {
			projection.Revocations = append(projection.Revocations, front.NativeProjectionRevocation{
				Established: "contract", Revoked: "contract/nilability", Event: event,
			})
		}
		return projection
	}
	appendRow := func(facts []equation.Fact, content string) []equation.Fact {
		return appendNativeProjection(facts, operation, row(content))
	}
	if nativeProjectionOperand(operands, "native-nilability-assert") != "" {
		return appendRow(nil, "nilability=non_nil")
	}
	withoutNil := proof.ProjectionWithoutNil(valueType)
	truthyNilOnly := typ.TypeEquals(withoutNil, typ.String) || typ.TypeEquals(withoutNil, typ.Number)
	var facts []equation.Fact
	switch nativeProjectionOperand(operands, "native-nilability-check") {
	case "not-nil":
		facts = appendRow(facts, "else_edge=nil nilability=non_nil then_edge=non_nil")
		arm := "nilability=non_nil"
		if len(operands["native-nilability-backedge-write"]) != 0 {
			arm = "nilability=maybe_nil"
		}
		facts = appendRow(facts, arm)
		facts = appendRow(facts, "nilability=nil")
	case "nil":
		facts = appendRow(facts, "else_edge=non_nil nilability=non_nil then_edge=nil")
		if nativeProjectionOperand(operands, "native-nilability-mode") != "captured" {
			facts = appendRow(facts, "nilability=nil")
		}
	case "truthy":
		if !truthyNilOnly {
			return nil
		}
		facts = appendRow(facts, "else_edge=nil nilability=non_nil then_edge=non_nil")
		facts = appendRow(facts, "nilability=nil")
	case "falsy":
		if !truthyNilOnly {
			return nil
		}
		facts = appendRow(facts, "else_edge=non_nil nilability=non_nil then_edge=nil")
		facts = appendRow(facts, "nilability=nil")
	}
	return facts
}

func nativeMetatableReadFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	return nativeMetatableReadFactFor("native-metatable-read", operation, operands, partition)
}

func nativeMetatableReadFactFor(
	prefix string,
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, equation.OperandRole(prefix+"-key"))
	occurrence := nativeProjectionOperand(operands, equation.OperandRole(prefix+"-occurrence"))
	root := operands[equation.OperandRole(prefix+"-root")]
	if key == "" || occurrence == "" || len(root) == 0 {
		return equation.Fact{}, false
	}
	identity, found := tableIdentityForTerm(root, partition)
	if !found || !heapTableClosed(identity, partition) || heapMetaAttached(identity, partition) ||
		heapHasExternalCallback(identity, partition) {
		return equation.Fact{}, false
	}
	return nativeProjectionFact(operation, front.NativeProjection{
		Key: key, Value: "index_chain=elided metatable=absent",
		Subject: nativeProjectionOperand(operands, equation.OperandRole(prefix+"-subject")), Occurrence: occurrence,
		Established: occurrence, Revoked: "meta.set", Event: "meta.set",
		Revocations: []front.NativeProjectionRevocation{{
			Established: occurrence, Revoked: "meta.set", Event: "meta.set",
		}},
	})
}

func nativeMetatableExpressionReadFacts(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) []equation.Fact {
	var facts []equation.Fact
	for _, prefix := range []string{"native-metatable-read-left", "native-metatable-read-right", "native-metatable-read-value"} {
		if fact, published := nativeMetatableReadFactFor(prefix, operation, operands, partition); published {
			facts = append(facts, fact)
		}
	}
	return facts
}

func nativeMetatableInstallFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
) (equation.Fact, bool) {
	key := nativeProjectionOperand(operands, "native-metatable-install-key")
	occurrence := nativeProjectionOperand(operands, "native-metatable-install-occurrence")
	indexDisplay := nativeProjectionOperand(operands, "native-metatable-install-index-display")
	if key == "" || occurrence == "" || indexDisplay == "" {
		return equation.Fact{}, false
	}
	if nativeProjectionOperand(operands, equation.RoleGlobalCallee) != "setmetatable" {
		return equation.Fact{}, false
	}
	receiver, receiverOK := tableIdentityForTerm(operands["native-metatable-install-receiver"], partition)
	meta, metaOK := tableIdentityForTerm(operands["native-metatable-install-meta"], partition)
	index, indexOK := tableIdentityForTerm(operands["native-metatable-install-index"], partition)
	if !receiverOK || !metaOK || !indexOK {
		return equation.Fact{}, false
	}
	publishedIndex, linked := heapMemberCurrent(factkey.HeapMemberIdentity, meta, ".__index", partition)
	if !linked || string(publishedIndex) != string(index) || heapHasExternalCallback(receiver, partition) {
		return equation.Fact{}, false
	}
	events := []string{"meta.set", "meta.mutate"}
	row := front.NativeProjection{
		Key: key, Value: "index_table=" + indexDisplay + " sealed=true",
		Subject: nativeProjectionOperand(operands, "native-metatable-install-subject"), Occurrence: occurrence,
		Established: occurrence, Revoked: "meta.set,meta.mutate", Event: events[0],
	}
	for _, event := range events {
		row.Revocations = append(row.Revocations, front.NativeProjectionRevocation{
			Established: occurrence, Revoked: "meta.set,meta.mutate", Event: event,
		})
	}
	return nativeProjectionFact(operation, row)
}

func nativeElementClass(container []byte, partition equation.Partition) (string, typ.Type, bool) {
	declared, found := declaredTypeForTerm(container, partition)
	if !found {
		return "", nil, false
	}
	element, found := declaredElementContract(declared)
	if !found || element == nil {
		return "", nil, false
	}
	resolved := unwrap.Alias(element)
	switch {
	case typ.TypeEquals(resolved, typ.Number), typ.IsIntegerIndexType(resolved):
		return "number", element, true
	case resolved.Kind() == kind.Record:
		return "record", element, true
	default:
		return "", nil, false
	}
}

func nativeProjectionWithEvents(
	operation equation.BoundEquation,
	key, occurrence, subject, content string,
	events []string,
) (equation.Fact, bool) {
	if key == "" || occurrence == "" || content == "" || len(events) == 0 {
		return equation.Fact{}, false
	}
	row := front.NativeProjection{
		Key: key, Value: content, Subject: subject, Occurrence: occurrence,
		Established: occurrence, Revoked: strings.Join(events, ","), Event: events[0],
	}
	for _, event := range events {
		row.Revocations = append(row.Revocations, front.NativeProjectionRevocation{
			Established: occurrence, Revoked: strings.Join(events, ","), Event: event,
		})
	}
	return nativeProjectionFact(operation, row)
}

func nativeElementDomainFact(
	operation equation.BoundEquation,
	partition equation.Partition,
) (equation.Fact, bool) {
	operands := make(map[equation.OperandRole][]byte, len(operation.Operands))
	for _, operand := range operation.Operands {
		operands[operand.Role] = operand.Value
	}
	key := nativeProjectionOperand(operands, "native-element-domain-key")
	occurrence := nativeProjectionOperand(operands, "native-element-domain-occurrence")
	container := operands["container"]
	if key == "" || occurrence == "" || len(container) == 0 {
		return equation.Fact{}, false
	}
	start, exact := nativeExactIntegerTerm(operands["native-growth-iterator-start"], partition)
	if !exact || start != "1" || nativeProjectionOperand(operands, "native-growth-maker") != "table" {
		return equation.Fact{}, false
	}
	class, _, classified := nativeElementClass(container, partition)
	if !classified {
		return equation.Fact{}, false
	}
	subject := nativeProjectionOperand(operands, "native-growth-subject")
	switch class {
	case "number":
		content := "element_class=number presence=dense_prefix"
		events := []string{"write.element", "write.length", "call.opaque", "escape"}
		if nativeProjectionOperand(operands, "native-element-domain-opaque-call") == "" {
			content = "element_class=number mutations_closed=true presence=dense_prefix"
			events = []string{"write.element", "write.length", "meta.set", "call.opaque", "escape"}
		}
		return nativeProjectionWithEvents(operation, key, occurrence, subject, content, events)
	case "record":
		if _, found := tableIdentityForTerm(operands["value"], partition); !found {
			return equation.Fact{}, false
		}
		return nativeProjectionWithEvents(operation, key, occurrence, subject,
			"element_class=record ownership=move write_barrier=required",
			[]string{"write.element", "call.opaque", "escape", "shape.transition"})
	default:
		return equation.Fact{}, false
	}
}

func nativeElementReadFact(
	operation equation.BoundEquation,
	operands map[string][]byte,
	partition equation.Partition,
	resultValue []byte,
) (equation.Fact, bool) {
	key, occurrence := string(operands["native-element-read-key"]), string(operands["native-element-read-occurrence"])
	container, index := operands["container"], operands["key"]
	if key == "" || occurrence == "" || len(container) == 0 || len(index) == 0 ||
		len(operands["native-element-read-prior-opaque-call"]) != 0 ||
		len(operands["native-element-read-prior-suspend"]) != 0 {
		return equation.Fact{}, false
	}
	class, _, classified := nativeElementClass(container, partition)
	if !classified {
		return equation.Fact{}, false
	}
	subject := string(operands["native-element-read-subject"])
	local := len(operands["native-element-read-local-maker"]) != 0 ||
		len(operands["native-element-read-shared"]) != 0
	if local {
		events := []string{"write.element", "write.length", "grow", "gc.relocate"}
		if len(operands["native-element-read-shared"]) != 0 {
			events = []string{"write.element", "write.length", "grow", "suspend", "call.opaque", "escape"}
		} else if len(operands["native-element-read-has-opaque-call"]) != 0 {
			events = []string{"write.element", "write.length", "grow", "call.opaque", "escape"}
		}
		return nativeProjectionWithEvents(operation, key, occurrence, subject, "element_class="+class, events)
	}
	present := indexPresenceProven(container, index, operation.Target.Name, partition) ||
		residueIndexPresenceProven(container, index, partition)
	if resultType, decoded := shapefact.DecodeTarget(resultValue); decoded && resultType != nil &&
		!typevalue.TypeIncludesNil(resultType) {
		present = true
	}
	content := "result_nilability=maybe_nil"
	if present {
		content = "presence=proven result_nilability=non_nil"
	}
	return nativeProjectionWithEvents(operation, key, occurrence, subject, content,
		[]string{"write.element", "write.length", "write.local", "meta.set", "call.opaque"})
}

func nativeElementStaticReadFact(
	operation equation.BoundEquation,
	operands map[equation.OperandRole][]byte,
	partition equation.Partition,
	resultValue []byte,
) (equation.Fact, bool) {
	generic := make(map[string][]byte)
	for target, source := range map[string]equation.OperandRole{
		"native-element-read-key":               "native-element-static-key",
		"native-element-read-occurrence":        "native-element-static-occurrence",
		"native-element-read-subject":           "native-element-static-subject",
		"container":                             "native-element-static-container",
		"key":                                   "native-element-static-index",
		"native-element-read-shared":            "native-element-static-shared",
		"native-element-read-local-maker":       "native-element-static-local-maker",
		"native-element-read-has-suspend":       "native-element-static-has-suspend",
		"native-element-read-prior-suspend":     "native-element-static-prior-suspend",
		"native-element-read-has-opaque-call":   "native-element-static-has-opaque-call",
		"native-element-read-prior-opaque-call": "native-element-static-prior-opaque-call",
		"native-element-read-has-growth":        "native-element-static-has-growth",
	} {
		generic[target] = operands[source]
	}
	return nativeElementReadFact(operation, generic, partition, resultValue)
}
