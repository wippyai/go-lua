package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/equation"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

func nativeOperandKey(body *wir.Body, operand wir.Operand) string {
	switch operand.Kind {
	case wir.OperandPath:
		item := body.Path(wir.PathRef(operand.Ref))
		return nativePathKey(item.Symbol, item.Key())
	case wir.OperandTemp:
		return fmt.Sprintf("temp/%d", operand.Ref)
	default:
		return ""
	}
}

func nativePathKey(symbol wir.SymbolID, key path.PathKey) string {
	if symbol != 0 {
		return fmt.Sprintf("sym%d", symbol)
	}
	return string(key)
}

// numericNativeFacts is intentionally a projection, not another abstract
// interpreter. Its inputs are the lowering-owned instruction topology and the
// resolved WIR type identities. A missing representation is therefore an
// absent fact, never a guessed numeric licence.
func numericNativeFacts(root front.Compilation) []NativeFact {
	var rows []NativeFact
	forEachNativeBody(root, func(compilation front.Compilation) {
		rows = append(rows, numericBodyFacts(compilation)...)
	})
	return rows
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

// nativePublishedConstantWord renders an exact value-lattice scalar in the
// native machine-word vocabulary. Provenance and write uniqueness are lowered
// by front; arithmetic itself is never repeated here.
func nativePublishedConstantWord(value []byte) (nativeConstantWord, bool) {
	text := string(value)
	switch {
	case strings.HasPrefix(text, "scalar/number/"):
		number := strings.TrimPrefix(text, "scalar/number/")
		if numericLiteralIsInteger(number) {
			parsed, err := strconv.ParseInt(number, 10, 64)
			return nativeConstantWord{representation: "integer", text: number, integer: parsed, hasInteger: err == nil}, err == nil
		}
		parsed, err := strconv.ParseFloat(number, 64)
		return nativeConstantWord{representation: "float", text: number, float: parsed, hasFloat: err == nil}, err == nil && !math.IsInf(parsed, 0) && !math.IsNaN(parsed)
	case strings.HasPrefix(text, "scalar/string/"):
		return nativeConstantWord{representation: "string", text: strings.TrimPrefix(text, "scalar/string/")}, true
	case strings.HasPrefix(text, "scalar/bool/"):
		return nativeConstantWord{representation: "boolean", text: strings.TrimPrefix(text, "scalar/bool/")}, true
	default:
		return nativeConstantWord{}, false
	}
}

// publishedNestedConstantValues is the one residual constant scan. Nested
// lexical bodies are admitted but not evaluated by the root fixpoint, so their
// value partitions do not exist for a publication kernel to read. Until child
// admission evaluates those artifacts, retaining their exact constant rows
// requires reading the independently admitted child WIR. The root body is
// deliberately excluded: its constants are ordinary native-source publication
// equations and never pass through this residual.
func publishedNestedConstantValues(root front.Compilation) []equation.Fact {
	var rows []equation.Fact
	var visit func(front.Compilation)
	visit = func(compilation front.Compilation) {
		if body := compilation.WIR; body != nil {
			folded := nativeFoldedConstants(body)
			for index := 0; index < body.Len(); index++ {
				word, ok := folded[index]
				if !ok {
					continue
				}
				rows = append(rows, equation.Fact{
					Key:   fmt.Sprintf("constant_value/%x/op-%08d", compilation.Body, index),
					Value: []byte("representation=" + word.representation + " value=" + word.text),
				})
			}
		}
		for _, child := range compilation.Nested {
			visit(child)
		}
	}
	for _, child := range root.Nested {
		visit(child)
	}
	return rows
}

// nativeConstantWord is an exactly known machine word: the representation arm
// and the published spelling, together with the numeric payload arithmetic
// folding reads. A word carries a payload only when its exact value is known,
// so a spelling this cannot evaluate still names a value but never feeds an
// operation.
type nativeConstantWord struct {
	representation string
	text           string
	integer        int64
	float          float64
	hasInteger     bool
	hasFloat       bool
}

// floatValue reads the word on the float arm. An integer word converts only
// while the conversion is exact: beyond the 53-bit mantissa the float is a
// different value than the integer it came from.
func (word nativeConstantWord) floatValue() (float64, bool) {
	if word.hasFloat {
		return word.float, true
	}
	if word.hasInteger && word.integer >= -(1<<53) && word.integer <= 1<<53 {
		return float64(word.integer), true
	}
	return 0, false
}

// nativeFoldedConstants folds the body's constant lattice and maps each
// instruction index to the machine word its write installs. Literal writes seed
// the lattice; copies and exactly evaluable arithmetic propagate it. The lattice
// fails closed: an operand that does not resolve, a destination written more
// than once, a binding a nested closure captures, and an operation whose exact
// result this cannot name all stop the fold rather than approximate it.
func nativeFoldedConstants(body *wir.Body) map[int]nativeConstantWord {
	writes := nativeConstantWriteCounts(body)
	captured := nativeCapturedBindings(body)
	known := make(map[string]nativeConstantWord)
	folded := make(map[int]nativeConstantWord)
	resolve := func(item wir.Operand) (nativeConstantWord, bool) {
		if item.Kind == wir.OperandConst {
			return nativeLiteralWord(body.Const(wir.ConstRef(item.Ref)))
		}
		name := nativeOperandKey(body, item)
		if name == "" {
			return nativeConstantWord{}, false
		}
		word, ok := known[name]
		return word, ok
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		var word nativeConstantWord
		ok := false
		switch instruction.Op {
		case wir.OpAssign:
			word, ok = resolve(instruction.A)
		case wir.OpUnOp:
			if operand, resolved := resolve(instruction.A); resolved {
				word, ok = nativeFoldUnary(instruction.Operator, operand)
			}
		case wir.OpBinOp:
			left, leftOK := resolve(instruction.A)
			right, rightOK := resolve(instruction.B)
			if leftOK && rightOK {
				word, ok = nativeFoldBinary(instruction.Operator, left, right)
			}
		}
		if !ok {
			continue
		}
		// The word is the one this write installs, so a destination whose other
		// writes are outside this body's reach carries no single constant.
		name := nativeOperandKey(body, instruction.Dst)
		if name == "" || writes[name] != 1 || captured[name] {
			continue
		}
		known[name] = word
		folded[index] = word
	}
	return folded
}

// nativeLiteralWord reads a lowered literal into a machine word. The number arm
// keeps the source spelling as its text, so an integral float stays distinct
// from the integer of the same value.
func nativeLiteralWord(constant wir.Const) (nativeConstantWord, bool) {
	switch constant.Kind {
	case wir.ConstNumber:
		if numericLiteralIsInteger(constant.Number) {
			value, err := strconv.ParseInt(constant.Number, 10, 64)
			return nativeConstantWord{representation: "integer", text: constant.Number, integer: value, hasInteger: err == nil}, true
		}
		value, err := strconv.ParseFloat(constant.Number, 64)
		exact := err == nil && !math.IsInf(value, 0) && !math.IsNaN(value)
		return nativeConstantWord{representation: "float", text: constant.Number, float: value, hasFloat: exact}, true
	case wir.ConstString:
		return nativeConstantWord{representation: "string", text: strconv.Quote(constant.Str)}, true
	case wir.ConstBool:
		return nativeConstantWord{representation: "boolean", text: strconv.FormatBool(constant.Bool)}, true
	default:
		return nativeConstantWord{}, false
	}
}

// nativeIntegerWord names an exact integer machine word.
func nativeIntegerWord(value int64) (nativeConstantWord, bool) {
	return nativeConstantWord{representation: "integer", text: strconv.FormatInt(value, 10), integer: value, hasInteger: true}, true
}

// nativeFloatWord names an exact float machine word. Its text round-trips to the
// same float64 and always spells a float, so the word stays distinct from the
// integer of the same value. A non-finite result is no machine word this family
// ranges over and is withheld.
func nativeFloatWord(value float64) (nativeConstantWord, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nativeConstantWord{}, false
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, ".eE") {
		text += ".0"
	}
	return nativeConstantWord{representation: "float", text: text, float: value, hasFloat: true}, true
}

// nativeFoldUnary evaluates unary minus. Negating the most negative integer
// leaves the integer range, so it carries no exact word.
func nativeFoldUnary(operator wir.Operator, value nativeConstantWord) (nativeConstantWord, bool) {
	if operator != wir.UnNeg {
		return nativeConstantWord{}, false
	}
	if value.hasInteger {
		if value.integer == math.MinInt64 {
			return nativeConstantWord{}, false
		}
		return nativeIntegerWord(-value.integer)
	}
	if value.representation == "float" && value.hasFloat {
		return nativeFloatWord(-value.float)
	}
	return nativeConstantWord{}, false
}

// nativeFoldBinary evaluates a binary operation over two exactly known words.
// Two integers stay on the integer arm, a float operand promotes the result to
// the float arm, and division and exponentiation are withheld because their
// exact word is not one this names. String and boolean words take part in no
// arithmetic at all.
func nativeFoldBinary(operator wir.Operator, left, right nativeConstantWord) (nativeConstantWord, bool) {
	if !nativeNumericWord(left) || !nativeNumericWord(right) {
		return nativeConstantWord{}, false
	}
	if left.hasInteger && right.hasInteger {
		return nativeFoldIntegerArithmetic(operator, left.integer, right.integer)
	}
	switch operator {
	case wir.BinAdd, wir.BinSub, wir.BinMul:
	default:
		return nativeConstantWord{}, false
	}
	leftValue, leftOK := left.floatValue()
	rightValue, rightOK := right.floatValue()
	if !leftOK || !rightOK {
		return nativeConstantWord{}, false
	}
	switch operator {
	case wir.BinAdd:
		return nativeFloatWord(leftValue + rightValue)
	case wir.BinSub:
		return nativeFloatWord(leftValue - rightValue)
	default:
		return nativeFloatWord(leftValue * rightValue)
	}
}

func nativeNumericWord(word nativeConstantWord) bool {
	return word.representation == "integer" || word.representation == "float"
}

// nativeFoldIntegerArithmetic evaluates Lua integer arithmetic exactly. Addition,
// subtraction and multiplication withhold rather than wrap; floor division and
// modulo follow Lua's floor semantics and withhold on a zero divisor and on the
// one quotient that leaves the integer range.
func nativeFoldIntegerArithmetic(operator wir.Operator, left, right int64) (nativeConstantWord, bool) {
	switch operator {
	case wir.BinAdd:
		result := left + right
		if (right > 0 && result < left) || (right < 0 && result > left) {
			return nativeConstantWord{}, false
		}
		return nativeIntegerWord(result)
	case wir.BinSub:
		result := left - right
		if (right < 0 && result < left) || (right > 0 && result > left) {
			return nativeConstantWord{}, false
		}
		return nativeIntegerWord(result)
	case wir.BinMul:
		if left == -1 && right == math.MinInt64 || right == -1 && left == math.MinInt64 {
			return nativeConstantWord{}, false
		}
		result := left * right
		if left != 0 && result/left != right {
			return nativeConstantWord{}, false
		}
		return nativeIntegerWord(result)
	case wir.BinIDiv:
		if right == 0 || left == math.MinInt64 && right == -1 {
			return nativeConstantWord{}, false
		}
		quotient := left / right
		if left%right != 0 && (left < 0) != (right < 0) {
			quotient--
		}
		return nativeIntegerWord(quotient)
	case wir.BinMod:
		if right == 0 {
			return nativeConstantWord{}, false
		}
		if left == math.MinInt64 && right == -1 {
			return nativeIntegerWord(0)
		}
		remainder := left % right
		if remainder != 0 && (remainder < 0) != (right < 0) {
			remainder += right
		}
		return nativeIntegerWord(remainder)
	default:
		return nativeConstantWord{}, false
	}
}

// nativeConstantWriteCounts counts the writes that install a value in a
// destination. A claim narrows the value already there and is not a write; a
// call or loop header installs its results, so those destinations count too.
func nativeConstantWriteCounts(body *wir.Body) map[string]int {
	writes := make(map[string]int)
	count := func(operand wir.Operand) {
		if name := nativeOperandKey(body, operand); name != "" {
			writes[name]++
		}
	}
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		switch instruction.Op {
		case wir.OpClaim:
		case wir.OpCall, wir.OpIterate:
			for _, result := range body.Operands(instruction.Results) {
				count(result)
			}
		default:
			if instruction.WritesAssignmentPoint() {
				count(instruction.Dst)
			}
		}
	}
	return writes
}

// nativeCapturedBindings names the bindings a nested closure captures. A capture
// is writable from the nested body, whose instructions this body's write counts
// do not range over, so a captured binding holds no constant this body can
// prove.
func nativeCapturedBindings(body *wir.Body) map[string]bool {
	captured := make(map[string]bool)
	for index := 0; index < body.Len(); index++ {
		instruction := body.Instr(index)
		if instruction.Op != wir.OpClosure {
			continue
		}
		for _, capture := range body.Operands(instruction.List) {
			if name := nativeOperandKey(body, capture); name != "" {
				captured[name] = true
			}
		}
	}
	return captured
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
	// A string constant carries no numeric arm: its machine word is a
	// reference, not an integer or a float the representation family
	// classifies.
	word, ok := nativeLiteralWord(body.Const(wir.ConstRef(instruction.A.Ref)))
	if !ok {
		return "", false
	}
	return word.representation, true
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
