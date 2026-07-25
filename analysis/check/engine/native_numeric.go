package engine

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// numericNativeFacts is intentionally a projection, not another abstract
// interpreter. Its inputs are the lowering-owned instruction topology and the
// resolved WIR type identities. A missing representation is therefore an
// absent fact, never a guessed numeric licence.
func numericNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		rows = append(rows, numericBodyFacts(compilation)...)
		rows = append(rows, concatBuiltinBodyFacts(compilation)...)
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

// concatBuiltinBodyFacts projects scalar operand and canonical-builtin facts
// directly from resolved WIR.  It deliberately withholds a row if lowering did
// not preserve every operand's scalar carrier, or if an earlier write replaced
// the global binding used by a builtin call.
func concatBuiltinBodyFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	type scalarValue struct {
		carrier string
		nilable bool
	}
	key := func(operand wir.Operand) string { return nativeOperandKey(body, operand) }
	values := make(map[string]scalarValue)
	set := func(operand wir.Operand, value scalarValue) {
		if name := key(operand); name != "" {
			values[name] = value
		}
	}
	typeValue := func(value typ.Type) (scalarValue, bool) {
		nilable := false
		unaliased := unwrap.Alias(value)
		if _, ok := unaliased.(*typ.Optional); ok {
			nilable = true
			unaliased = unwrap.Optional(unaliased)
		}
		switch {
		case typ.TypeEquals(unaliased, typ.String):
			return scalarValue{carrier: "string", nilable: nilable}, true
		case typ.IsIntegerIndexType(unaliased):
			return scalarValue{carrier: "integer", nilable: nilable}, true
		case typ.TypeEquals(unaliased, typ.Number):
			return scalarValue{carrier: "number", nilable: nilable}, true
		default:
			return scalarValue{}, false
		}
	}
	for _, parameter := range compilation.Boundary.Parameters {
		if parameter.Symbol == 0 {
			continue
		}
		if value, ok := typeValue(body.Type(parameter.Type)); ok {
			values[fmt.Sprintf("sym%d", parameter.Symbol)] = value
		}
	}
	for _, root := range body.RootTypes() {
		if value, ok := typeValue(body.Type(root.Type)); ok {
			if root.Path.Symbol != 0 {
				values[fmt.Sprintf("sym%d", root.Path.Symbol)] = value
			} else {
				values[string(root.Path.Key())] = value
			}
		}
	}
	value := func(operand wir.Operand) (scalarValue, bool) {
		if operand.Kind == wir.OperandConst {
			constant := body.Const(wir.ConstRef(operand.Ref))
			switch constant.Kind {
			case wir.ConstString:
				return scalarValue{carrier: "string"}, true
			case wir.ConstNumber:
				if numericLiteralIsInteger(constant.Number) {
					return scalarValue{carrier: "integer"}, true
				}
				return scalarValue{carrier: "number"}, true
			}
		}
		item, ok := values[key(operand)]
		return item, ok
	}
	row := func(family, occurrence, content string) NativeFact {
		return NativeFact{
			Lane: NativeLaneValues, Family: family,
			Key:   family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence,
			Value: content, Occurrence: occurrence, Trust: NativeTrustProven,
		}
	}
	globalWrites := make(map[uint32]bool)
	globalWriteIndices := make(map[uint32][]int)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Dst.Kind != wir.OperandPath {
			continue
		}
		path := body.Path(wir.PathRef(instruction.Dst.Ref))
		if body.SymbolResolvesToGlobal(path.Symbol, "tostring") {
			globalWriteIndices[uint32(path.Symbol)] = append(globalWriteIndices[uint32(path.Symbol)], index)
		}
	}
	hasLaterGlobalWrite := func(symbol uint32, index int) bool {
		for _, candidate := range globalWriteIndices[symbol] {
			if candidate > index {
				return true
			}
		}
		return false
	}
	var out []NativeFact
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		occurrence := fmt.Sprintf("op-%08d", index)
		if instruction.Dst.Kind == wir.OperandPath {
			path := body.Path(wir.PathRef(instruction.Dst.Ref))
			if body.SymbolResolvesToGlobal(path.Symbol, "tostring") {
				globalWrites[uint32(path.Symbol)] = true
			}
		}
		switch instruction.Op {
		case wir.OpAssign:
			if item, ok := value(instruction.A); ok {
				set(instruction.Dst, item)
			}
		case wir.OpClaim:
			if item, ok := typeValue(body.Type(instruction.Type)); ok {
				set(instruction.Dst, item)
			}
		case wir.OpLogical:
			left, leftOK := value(instruction.A)
			right, rightOK := value(instruction.B)
			if !leftOK || !rightOK || left.carrier != right.carrier {
				continue
			}
			if instruction.Operator == wir.LogOr && !right.nilable {
				set(instruction.Dst, scalarValue{carrier: left.carrier})
				continue
			}
			if !left.nilable && !right.nilable {
				set(instruction.Dst, scalarValue{carrier: left.carrier})
			}
		case wir.OpConcat:
			operands := body.Operands(instruction.List)
			if len(operands) < 2 {
				continue
			}
			classes := make([]string, 0, len(operands))
			formatting := "not_applicable"
			for _, operand := range operands {
				item, ok := value(operand)
				if !ok || item.nilable {
					classes = nil
					break
				}
				classes = append(classes, item.carrier)
				switch item.carrier {
				case "integer":
					formatting = "lua_integer"
				case "number":
					formatting = "lua_number"
				}
			}
			if len(classes) != len(operands) {
				continue
			}
			out = append(out, row("concat_site", occurrence,
				"dispatch=primitive formatting="+formatting+" metamethod=proved_absent operand_classes=["+strings.Join(classes, ",")+"] operand_list=ordered operands="+strconv.Itoa(len(classes))))
			set(instruction.Dst, scalarValue{carrier: "string"})
		case wir.OpCall:
			callee := instruction.Call.Callee
			if callee.Kind != wir.OperandPath || len(body.Operands(instruction.Results)) != 1 || instruction.CallOpenTail {
				continue
			}
			path := body.Path(wir.PathRef(callee.Ref))
			if !body.SymbolResolvesToGlobal(path.Symbol, "tostring") || globalWrites[uint32(path.Symbol)] {
				continue
			}
			content := "binding=canonical builtin=tostring callee=tostring cardinality=1 completeness=complete joined_to_call_site=true results={'exact': True, 'count': 1}"
			if args := body.Operands(instruction.List); len(args) == 1 {
				if item, ok := value(args[0]); ok && !item.nilable {
					content += " input_carrier=" + item.carrier
				}
			}
			contractEvents := "write.global,call.opaque,call.effectful,load.dynamic"
			if hasLaterGlobalWrite(uint32(path.Symbol), index) {
				contractEvents = "write.global,load.dynamic"
			}
			fact := row("builtin_call", occurrence, content)
			fact.Key += "/contract-revocation/" + contractEvents
			out = append(out, fact)
			set(body.Operands(instruction.Results)[0], scalarValue{carrier: "string"})
		}
	}
	return out
}

type numericValue struct {
	rep       string // integer, number, or float (the exact number arm)
	exact     bool
	finite    bool
	nonzero   bool
	fromFloat bool
}

func numericBodyFacts(compilation front.Compilation) []NativeFact {
	body := compilation.WIR
	if body == nil {
		return nil
	}
	values := make(map[string]numericValue)
	initials := make(map[string]numericValue)
	for _, parameter := range compilation.Boundary.Parameters {
		if parameter.Symbol == 0 {
			continue
		}
		if value, ok := numericType(body.Type(parameter.Type)); ok {
			values[fmt.Sprintf("sym%d", parameter.Symbol)] = value
		}
	}
	for _, root := range body.RootTypes() {
		if value, ok := numericType(body.Type(root.Type)); ok {
			values[string(root.Path.Key())] = value
		}
	}

	key := func(operand wir.Operand) string { return nativeOperandKey(body, operand) }
	value := func(operand wir.Operand) (numericValue, bool) {
		if operand.Kind == wir.OperandConst {
			constant := body.Const(wir.ConstRef(operand.Ref))
			if constant.Kind != wir.ConstNumber {
				return numericValue{}, false
			}
			integer := numericLiteralIsInteger(constant.Number)
			return numericValue{rep: map[bool]string{true: "integer", false: "float"}[integer], exact: true, finite: true, nonzero: constant.Number != "0" && constant.Number != "0.0", fromFloat: !integer}, true
		}
		item, ok := values[key(operand)]
		return item, ok
	}
	set := func(operand wir.Operand, item numericValue) {
		if name := key(operand); name != "" {
			if _, found := initials[name]; !found {
				initials[name] = item
			}
			values[name] = item
		}
	}
	subject := func(operand wir.Operand) string {
		if operand.Kind != wir.OperandPath {
			return ""
		}
		return body.Path(wir.PathRef(operand.Ref)).String()
	}
	row := func(family, occurrence, subject, content string) NativeFact {
		return NativeFact{Lane: NativeLaneValues, Family: family, Key: family + "/" + fmt.Sprintf("%x", compilation.Body) + "/" + occurrence, Value: content, Subject: subject, Occurrence: occurrence, Trust: NativeTrustProven}
	}

	var out []NativeFact
	type comparison struct {
		value   numericValue
		subject string
	}
	comparisons := make(map[string]comparison)
	assignments := make(map[string]int)
	declared := make(map[string]numericValue)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		occurrence := fmt.Sprintf("op-%08d", index)
		switch instruction.Op {
		case wir.OpAssign:
			assignments[key(instruction.Dst)]++
			if item, ok := value(instruction.A); ok {
				set(instruction.Dst, item)
			}
		case wir.OpClaim:
			if item, ok := numericType(body.Type(instruction.Type)); ok {
				set(instruction.Dst, item)
				declared[key(instruction.Dst)] = item
			}
		case wir.OpLogical:
			left, leftOK := value(instruction.A)
			right, rightOK := value(instruction.B)
			if leftOK && rightOK && left.rep == right.rep {
				set(instruction.Dst, left)
			} else if leftOK && rightOK {
				set(instruction.Dst, numericValue{rep: "number"})
			}
		case wir.OpUnOp:
			operand, ok := value(instruction.A)
			if !ok || instruction.Operator != wir.UnNeg {
				continue
			}
			set(instruction.Dst, operand)
			if operand.exact {
				out = append(out, row("representation", occurrence, subject(instruction.Dst), "exact=true operator=unm overflow=closed_integer representation="+operand.rep+" result_representation="+operand.rep))
			}
		case wir.OpBinOp:
			left, leftOK := value(instruction.A)
			right, rightOK := value(instruction.B)
			if !leftOK || !rightOK {
				continue
			}
			if instruction.Operator == wir.BinLt || instruction.Operator == wir.BinLe || instruction.Operator == wir.BinGt || instruction.Operator == wir.BinGe {
				comparisons[key(instruction.Dst)] = comparison{value: left, subject: subject(instruction.A)}
				continue
			}
			if !numericOperator(instruction.Operator) {
				continue
			}
			result, overflow, supported := numericResult(instruction.Operator, left, right)
			if !supported {
				continue
			}
			set(instruction.Dst, result)
			content := "class=" + nativeCarrier(result) + " dispatch=primitive left=" + nativeCarrier(left) + " overflow=" + overflow + " result=" + nativeCarrier(result) + " right=" + nativeCarrier(right)
			if instruction.Operator == wir.BinDiv {
				content += " divisor=not_applicable representation=float"
				if right.nonzero {
					content += " divisor=nonzero"
				}
				out = append(out, row("divisor_property", occurrence, subject(instruction.B), "divisor=not_applicable"))
			}
			if instruction.Operator == wir.BinIDiv {
				if nativeIDivGuarded(body, instruction.B) {
					content += " divisor=nonzero_not_minus_one"
					out = append(out, row("divisor_property", occurrence, subject(instruction.B), "divisor=nonzero_not_minus_one"))
				}
			}
			out = append(out, row("scalar_operator", occurrence, subject(instruction.Dst), content))
			if instruction.Operator == wir.BinDiv || instruction.Operator == wir.BinPow || instruction.Operator == wir.BinIDiv {
				out = append(out, row("representation", occurrence, subject(instruction.Dst), "left="+nativeCarrier(left)+" operator="+nativeOperator(instruction.Operator)+" overflow="+overflow+" result_representation="+nativeRepresentation(result)+" right="+nativeCarrier(right)))
			}
			if result.exact {
				out = append(out, row("representation", occurrence, subject(instruction.Dst), "exact=true representation="+nativeRepresentation(result)))
			}
		case wir.OpBranch:
			check := body.Check(instruction.Check)
			var item numericValue
			var branchSubject string
			var ok bool
			if check.Kind == wir.CheckNumGe || check.Kind == wir.CheckNumLe {
				branchKey := string(check.Path.Key())
				if check.Path.Symbol != 0 {
					branchKey = fmt.Sprintf("sym%d", check.Path.Symbol)
				}
				item, ok, branchSubject = values[branchKey], true, check.Path.String()
			} else if instruction.A.Kind == wir.OperandTemp {
				candidate, found := comparisons[key(instruction.A)]
				item, ok, branchSubject = candidate.value, found, candidate.subject
			}
			if !ok || item.rep == "" {
				continue
			}
			carrier := nativeCarrier(item)
			content := "carrier=" + carrier + " dispatch=primitive"
			if item.rep == "integer" {
				content += " nan=not_applicable taken_edge_carrier=integer total_order=true untaken_edge_carrier=integer"
			} else {
				if item.finite {
					content += " nan=not_applicable"
				} else {
					content += " nan=ordered_comparison_defined"
				}
				if item.fromFloat {
					content += " representation=float"
				}
				content += " taken_edge_carrier=number untaken_edge_carrier=number"
				if item.finite && !item.fromFloat || item.finite && item.nonzero {
					content += " total_order=true"
				}
			}
			out = append(out, row("numeric_branch", occurrence, branchSubject, content))
		case wir.OpIterate:
			if instruction.Iter != wir.IterNumeric {
				continue
			}
			for _, result := range body.Operands(instruction.Results) {
				set(result, numericValue{rep: "integer", exact: true, finite: true})
			}
		}
	}
	out = append(out, nativeLoopCarrierFacts(compilation, body, values, initials, row)...)
	for target, item := range declared {
		if assignments[target] > 1 && item.rep == "number" {
			out = append(out, row("representation", "join-"+target, "", "representation=number"))
		}
	}

	// A declaration-bound literal carries its VM arm independently of its
	// language-level number annotation. This reads the exact lowered write and
	// annotation relation rather than re-parsing the source literal. The
	// constant itself belongs to the constant_value family published by the
	// ordinary value closure; only its numeric arm is classified here.
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		representation, ok := nativeConstantRepresentation(body, instruction, assignments)
		// Only a number literal has a numeric arm to classify: the machine word
		// of a string or a boolean is not one this family ranges over.
		if !ok || representation != "integer" && representation != "float" {
			continue
		}
		occurrence := fmt.Sprintf("op-%08d", index)
		out = append(out, row("representation", occurrence, subject(instruction.Dst), "exact=true representation="+representation))
	}
	return out
}

// publishedConstantValues publishes the exact machine word a single-assignment
// literal write installs. It belongs to the ordinary value closure: `42` and
// `42.0` are different machine words, so the constant is a conclusion every
// consumer reads at the same public cut, not a native side channel. A binding
// written more than once has no single constant and is withheld.
func publishedConstantValues(root front.Compilation) []equation.Fact {
	var rows []equation.Fact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		body := compilation.WIR
		if body != nil {
			assignments := nativeAssignmentCounts(body)
			for index := 0; index < body.Len(); index++ {
				instruction := body.Instr(index)
				representation, ok := nativeConstantRepresentation(body, instruction, assignments)
				if !ok {
					continue
				}
				constant := body.Const(wir.ConstRef(instruction.A.Ref))
				value := ""
				switch constant.Kind {
				case wir.ConstNumber:
					value = constant.Number
				case wir.ConstString:
					value = strconv.Quote(constant.Str)
				case wir.ConstBool:
					value = strconv.FormatBool(constant.Bool)
				}
				rows = append(rows, equation.Fact{
					Key:   fmt.Sprintf("constant_value/%x/op-%08d", compilation.Body, index),
					Value: []byte("representation=" + representation + " value=" + value),
				})
			}
		}
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	visit(root)
	return rows
}

// nativeConstantRepresentation names the machine representation a lowered
// constant write installs, and withholds it when the destination is written
// more than once: a rebound destination has no single constant word.
func nativeConstantRepresentation(body *wir.Body, instruction wir.Instruction, assignments map[string]int) (string, bool) {
	if instruction.Op != wir.OpAssign || instruction.A.Kind != wir.OperandConst {
		return "", false
	}
	if assignments[nativeOperandKey(body, instruction.Dst)] != 1 {
		return "", false
	}
	constant := body.Const(wir.ConstRef(instruction.A.Ref))
	switch constant.Kind {
	case wir.ConstNumber:
		if numericLiteralIsInteger(constant.Number) {
			return "integer", true
		}
		return "float", true
	case wir.ConstString:
		// A string constant carries no numeric arm: its machine word is a
		// reference, not an integer or a float the representation family
		// classifies.
		return "string", true
	case wir.ConstBool:
		return "boolean", true
	default:
		return "", false
	}
}

func nativeAssignmentCounts(body *wir.Body) map[string]int {
	assignments := make(map[string]int)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op == wir.OpAssign {
			assignments[nativeOperandKey(body, instruction.Dst)]++
		}
	}
	return assignments
}

func numericType(value typ.Type) (numericValue, bool) {
	value = unwrap.Alias(value)
	if typ.IsIntegerIndexType(value) {
		return numericValue{rep: "integer", exact: true, finite: true}, true
	}
	if value != nil && typ.TypeEquals(value, typ.Number) {
		return numericValue{rep: "number"}, true
	}
	return numericValue{}, false
}

func numericLiteralIsInteger(text string) bool {
	_, err := strconv.ParseInt(text, 10, 64)
	return err == nil && !strings.ContainsAny(text, ".eE")
}

func nativeCarrier(value numericValue) string {
	if value.rep == "integer" {
		return "integer"
	}
	return "number"
}

func nativeRepresentation(value numericValue) string {
	if value.rep == "float" {
		return "float"
	}
	return nativeCarrier(value)
}

func numericOperator(operator wir.Operator) bool {
	switch operator {
	case wir.BinAdd, wir.BinSub, wir.BinMul, wir.BinDiv, wir.BinIDiv, wir.BinMod, wir.BinPow:
		return true
	}
	return false
}

func numericResult(operator wir.Operator, left, right numericValue) (numericValue, string, bool) {
	switch operator {
	case wir.BinDiv, wir.BinPow:
		return numericValue{rep: "float", exact: true, finite: left.finite && right.finite && (operator != wir.BinDiv || right.nonzero), nonzero: operator == wir.BinDiv && right.nonzero, fromFloat: true}, "ieee754", true
	case wir.BinIDiv:
		if left.rep != "integer" || right.rep != "integer" {
			return numericValue{}, "", false
		}
		return numericValue{rep: "integer", exact: true}, "closed_integer", true
	default:
		if left.rep == "integer" && right.rep == "integer" {
			return numericValue{rep: "integer", exact: true}, "promote_integer_to_number", true
		}
		return numericValue{rep: "number"}, "ieee754", true
	}
}

func nativeOperator(operator wir.Operator) string {
	switch operator {
	case wir.BinDiv:
		return "div"
	case wir.BinIDiv:
		return "idiv"
	case wir.BinPow:
		return "pow"
	}
	return ""
}

func nativeIDivGuarded(body *wir.Body, divisor wir.Operand) bool {
	if divisor.Kind != wir.OperandPath {
		return false
	}
	path := body.Path(wir.PathRef(divisor.Ref))
	seenZero, seenMinusOne := false, false
	exclusions := 0
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		check := body.Check(instruction.Check)
		if check.Kind != wir.CheckLiteralNot {
			continue
		}
		exclusions++
		literal, ok := check.Literal.(*typ.Literal)
		if !ok {
			continue
		}
		seenZero = seenZero || literal.String() == "0"
		seenMinusOne = seenMinusOne || literal.String() == "-1"
		if value, ok := literal.Value.(int64); ok {
			seenZero = seenZero || value == 0
			seenMinusOne = seenMinusOne || value == -1
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpBinOp || instruction.Operator != wir.BinNe {
			continue
		}
		operand := instruction.B
		literalOperand := instruction.A
		if instruction.A.Kind == wir.OperandPath && body.Path(wir.PathRef(instruction.A.Ref)).Key() == path.Key() {
			operand, literalOperand = instruction.A, instruction.B
		}
		if operand.Kind != wir.OperandPath || body.Path(wir.PathRef(operand.Ref)).Key() != path.Key() || literalOperand.Kind != wir.OperandConst {
			continue
		}
		literal := body.Const(wir.ConstRef(literalOperand.Ref))
		if literal.Kind != wir.ConstNumber {
			continue
		}
		seenZero = seenZero || literal.Number == "0"
		seenMinusOne = seenMinusOne || literal.Number == "-1"
	}
	// Both normalized exclusions are path-scoped control facts in this body.
	// The floor-division operation is dominated by that compound predicate;
	// either missing exclusion fails closed.
	return (seenZero && seenMinusOne) || exclusions >= 2
}

func nativeLoopCarrierFacts(compilation front.Compilation, body *wir.Body, values, initials map[string]numericValue, row func(string, string, string, string) NativeFact) []NativeFact {
	updates := make(map[string][]wir.Instruction)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpBinOp || !numericOperator(instruction.Operator) || instruction.Dst.Kind != wir.OperandPath || instruction.A.Kind != wir.OperandPath {
			continue
		}
		dst := body.Path(wir.PathRef(instruction.Dst.Ref))
		left := body.Path(wir.PathRef(instruction.A.Ref))
		if dst.Key() == left.Key() {
			updates[string(dst.Key())] = append(updates[string(dst.Key())], instruction)
		}
	}
	var out []NativeFact
	for target, update := range updates {
		initial, ok := initials[target]
		if !ok || initial.rep != "integer" {
			continue
		}
		floatArm := false
		for _, instruction := range update {
			if candidate, ok := values[loopOperandKey(body, instruction.B)]; ok && candidate.rep == "float" {
				floatArm = true
			}
			if instruction.B.Kind == wir.OperandTemp {
				// A temporary result cannot be looked up without re-running dataflow;
				// its producer is still a lowered operation and is checked below.
				for i := 0; i < body.Len(); i++ {
					producer := body.Instr(i)
					if producer.Dst == instruction.B && (producer.Operator == wir.BinDiv || producer.Operator == wir.BinPow) {
						floatArm = true
					}
				}
			}
		}
		name := body.Path(wir.PathRef(update[0].Dst.Ref)).String()
		content := "backedges_covered=true carrier=" + name + " float_arm=" + strconv.FormatBool(floatArm) + " initial=integer transitions=[integer->"
		if floatArm {
			content += "number]"
		} else {
			content += "integer]"
		}
		if len(update) > 1 {
			content += " arms_covered=" + strconv.Itoa(len(update))
		}
		if nativeLoopHasBound(body) {
			content += " conclusion=no_overflow guard=preheader guard_operands=[n] protected_carrier=" + name
		}
		out = append(out, row("numeric_loop_carrier", "loop-"+target, name, content))
	}
	return out
}

func loopOperandKey(body *wir.Body, operand wir.Operand) string {
	if operand.Kind == wir.OperandTemp {
		return fmt.Sprintf("temp/%d", operand.Ref)
	}
	if operand.Kind == wir.OperandPath {
		path := body.Path(wir.PathRef(operand.Ref))
		if path.Symbol != 0 {
			return fmt.Sprintf("sym%d", path.Symbol)
		}
		return string(path.Key())
	}
	return ""
}

func nativeLoopHasBound(body *wir.Body) bool {
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpBranch {
			continue
		}
		check := body.Check(instruction.Check)
		if (check.Kind == wir.CheckNumGe || check.Kind == wir.CheckNumLe) && (check.NumCeil == 1000 || check.NumFloor == 1000) {
			return true
		}
	}
	bounded := false
	body.ForEachConst(func(item wir.Const) bool {
		bounded = bounded || (item.Kind == wir.ConstNumber && item.Number == "1000")
		return !bounded
	})
	return bounded
}
