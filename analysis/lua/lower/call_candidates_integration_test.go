package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func requireExecutableOperand(t *testing.T, p *program.Program, owner, operand keyspace.Term) {
	t.Helper()
	flow := p.Flow()
	if operand == 0 || flow.Containment().Static(operand) {
		return
	}
	if !flow.Executable().Contains(operand) {
		t.Fatalf("Executable(%v) has non-executable operand %v", owner, operand)
	}
}

// This is the vertical law for the one Program runtime-admission authority.
// It deliberately combines a static typeof(require(...)) tree with live
// aggregates and constructors, so static syntax must stay absent while every
// may-evaluated runtime operand is closed into Executable.
func TestExecutableClosesRuntimeConstructorOperands(t *testing.T) {
	p := parseBindLower(t, `
type Snapshot = typeof(require("dependency"))
local api = require("dependency")
type Subject = api.Schema.User
local key = "item"
local value = -(api[key] + 1)
local result = value and api(key, { [key] = value, value })
return result
`)
	staticImport, staticOK := p.Module().ImportAt(0)
	liveImport, liveOK := p.Module().ImportAt(1)
	staticCall := staticImport.Call
	liveCall := liveImport.Call
	flow := p.Flow()
	if !staticOK || !flow.Containment().Static(staticCall) || flow.Executable().Contains(staticCall) {
		t.Fatalf("static require Call = %v/%v, want static and non-executable", staticCall, staticOK)
	}
	if !liveOK || flow.Containment().Static(liveCall) || !flow.Executable().Contains(liveCall) {
		t.Fatalf("live require Call = %v/%v, want executable runtime occurrence", liveCall, liveOK)
	}
	valuesView := flow.Authored().Values()
	for index := 0; index < valuesView.Count(); index++ {
		values, _ := valuesView.At(index)
		if !flow.Executable().Contains(values) {
			continue
		}
		count, _ := valuesView.Len(values)
		for member := 0; member < count; member++ {
			value, _ := valuesView.Member(values, member)
			requireExecutableOperand(t, p, values, value)
		}
		_, tail, _ := valuesView.Get(values)
		requireExecutableOperand(t, p, values, tail)
	}
	calls := flow.Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		if !flow.Executable().Contains(call) {
			continue
		}
		_, callee, _, actuals, _ := calls.Get(call)
		requireExecutableOperand(t, p, call, callee)
		requireExecutableOperand(t, p, call, actuals)
	}
	unaries := flow.Authored().Operators().Unaries()
	for index := 0; index < unaries.Count(); index++ {
		term, _ := unaries.At(index)
		if flow.Executable().Contains(term) {
			_, _, operand, _ := unaries.Get(term)
			requireExecutableOperand(t, p, term, operand)
		}
	}
	binaries := flow.Authored().Operators().Binaries()
	for index := 0; index < binaries.Count(); index++ {
		term, _ := binaries.At(index)
		if flow.Executable().Contains(term) {
			_, _, left, right, _ := binaries.Get(term)
			requireExecutableOperand(t, p, term, left)
			requireExecutableOperand(t, p, term, right)
		}
	}
	selects := flow.Authored().Operators().Selects()
	for index := 0; index < selects.Count(); index++ {
		term, _ := selects.At(index)
		if flow.Executable().Contains(term) {
			_, _, left, right, _ := selects.Get(term)
			requireExecutableOperand(t, p, term, left)
			requireExecutableOperand(t, p, term, right)
		}
	}
	tables := flow.Authored().Tables()
	for index := 0; index < tables.Count(); index++ {
		table, _ := tables.At(index)
		if !flow.Executable().Contains(table) {
			continue
		}
		count, _ := tables.FieldCount(table)
		for fieldIndex := 0; fieldIndex < count; fieldIndex++ {
			field, _ := tables.FieldAt(table, fieldIndex)
			requireExecutableOperand(t, p, table, field)
		}
	}
	fields := flow.Authored().Fields()
	for index := 0; index < fields.Count(); index++ {
		field, _ := fields.At(index)
		if !flow.Executable().Contains(field) {
			continue
		}
		_, key, values, fieldKind, _ := fields.Get(field)
		if fieldKind == kind.FieldKey || fieldKind == kind.FieldExact {
			requireExecutableOperand(t, p, field, key)
		}
		requireExecutableOperand(t, p, field, values)
	}
	typeOfs := p.Static().Operators().TypeOfs()
	for index := 0; index < typeOfs.Count(); index++ {
		typeOf, _ := typeOfs.At(index)
		if flow.Executable().Contains(typeOf) {
			t.Fatalf("static TypeOf %v became executable", typeOf)
		}
	}
}

func TestFlowCandidatesClassifyAuthoredOperatorFamilies(t *testing.T) {
	p := parseBindLower(t, "\nlocal function candidates(a, b, ...)\n  local unaryNeg = -a\n  local unaryNot = not a\n  local unaryLen = #a\n  local unaryBit = ~a\n  local add = a + b\n  local sub = a - b\n  local mul = a * b\n  local div = a / b\n  local idiv = a // b\n  local mod = a % b\n  local pow = a ^ b\n  local concat = a .. b\n  local band = a & b\n  local bor = a | b\n  local bxor = a ~ b\n  local shl = a << b\n  local shr = a >> b\n  local equal = a == b\n  local notEqual = a ~= b\n  local less = a < b\n  local lessEqual = a <= b\n  local greater = a > b\n  local greaterEqual = a >= b\n  local got = a[b]\n  a[b] = b\n  a:b(b, ...)\n  return a(b, ...)\nend\n")
	candidates := p.Flow().Candidates()
	unary := candidates.Unary()
	binary := candidates.Binary()
	access := candidates.Access()
	if unary.NumericCount() != 2 || unary.LengthCount() != 1 {
		t.Fatalf("Unary candidates numeric/length = %d/%d, want 2/1", unary.NumericCount(), unary.LengthCount())
	}
	if binary.ArithmeticCount() != 7 || binary.BitwiseCount() != 5 || binary.ConcatCount() != 1 || binary.EqualityCount() != 2 || binary.OrderCount() != 4 {
		t.Fatalf("Binary candidates = arithmetic %d bitwise %d concat %d equality %d order %d", binary.ArithmeticCount(), binary.BitwiseCount(), binary.ConcatCount(), binary.EqualityCount(), binary.OrderCount())
	}
	// The authored method call reads its receiver's field before invoking it,
	// so it contributes a second IndexGet alongside the explicit a[b] read.
	if access.GetCount() != 2 || access.SetCount() != 1 {
		t.Fatalf("Access candidates get/set = %d/%d, want 2/1 (explicit read plus method-callee read)", access.GetCount(), access.SetCount())
	}
	neg, _ := unary.NumericAt(0)
	_, op, _, negOK := p.Flow().Authored().Operators().Unaries().Get(neg)
	if !negOK || op != kind.UnaryNeg {
		t.Fatalf("first numeric candidate Unary op = %v/%v, want neg", op, negOK)
	}
	add, _ := binary.ArithmeticAt(0)
	_, addOp, _, _, addOK := p.Flow().Authored().Operators().Binaries().Get(add)
	if !addOK || addOp != kind.BinaryAdd {
		t.Fatalf("first arithmetic candidate op = %v/%v, want add", addOp, addOK)
	}
}

func TestFlowCandidatesDoNotClassifyStaticAuthoredRows(t *testing.T) {
	p := parseBindLower(t, "type Snapshot = typeof(function(a, b) return -a, #a, a + b, a[b], a(b) end)")
	candidates := p.Flow().Candidates()
	if candidates.Unary().NumericCount() != 0 || candidates.Binary().ArithmeticCount() != 0 || candidates.Access().GetCount() != 0 {
		t.Fatalf("static rows entered runtime candidates: unary=%d binary=%d access=%d", candidates.Unary().NumericCount(), candidates.Binary().ArithmeticCount(), candidates.Access().GetCount())
	}
	for index := 0; index < p.Flow().Authored().Operators().Unaries().Count(); index++ {
		term, _ := p.Flow().Authored().Operators().Unaries().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Unary %v escaped static containment", term)
		}
	}
	for index := 0; index < p.Flow().Authored().Operators().Binaries().Count(); index++ {
		term, _ := p.Flow().Authored().Operators().Binaries().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Binary %v escaped static containment", term)
		}
	}
	for index := 0; index < p.Flow().Authored().Calls().Count(); index++ {
		term, _ := p.Flow().Authored().Calls().At(index)
		if !p.Flow().Containment().Static(term) {
			t.Fatalf("static Call %v escaped static containment", term)
		}
	}
}

func TestProgramNumericCandidateVocabularyAndSources(t *testing.T) {
	p := parseBindLower(t, `
local a, b = 1, 2
local n = -a
return a + b, a - b, a * b, a / b, a // b, a % b, a ^ b,
  a .. b, a & b, a | b, a ~ b, a << b, a >> b,
  a == b, a ~= b, a < b, a <= b, a > b, a >= b, n
`)
	flow := p.Flow()
	binaries := flow.Authored().Operators().Binaries()
	unaries := flow.Authored().Operators().Unaries()
	binaryCandidates := flow.Candidates().Binary()

	assertBinaryBucket := func(name string, count int, at func(int) (keyspace.Term, bool), valid func(kind.BinaryOp) bool) {
		t.Helper()
		for index := 0; index < count; index++ {
			term, ok := at(index)
			if !ok || !flow.Executable().Contains(term) {
				t.Fatalf("%s candidate %d = %v/%v", name, index, term, ok)
			}
			_, op, left, right, rowOK := binaries.Get(term)
			if !rowOK || left == 0 || right == 0 || !valid(op) {
				t.Fatalf("%s candidate %d source = op %v left %v right %v ok %v", name, index, op, left, right, rowOK)
			}
		}
	}

	assertBinaryBucket("arithmetic", binaryCandidates.ArithmeticCount(), binaryCandidates.ArithmeticAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryAdd && op <= kind.BinaryPow
	})
	assertBinaryBucket("concat", binaryCandidates.ConcatCount(), binaryCandidates.ConcatAt, func(op kind.BinaryOp) bool {
		return op == kind.BinaryConcat
	})
	assertBinaryBucket("bitwise", binaryCandidates.BitwiseCount(), binaryCandidates.BitwiseAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryBitAnd && op <= kind.BinaryShiftRight
	})
	assertBinaryBucket("equality", binaryCandidates.EqualityCount(), binaryCandidates.EqualityAt, func(op kind.BinaryOp) bool {
		return op == kind.BinaryEqual || op == kind.BinaryNotEqual
	})
	assertBinaryBucket("order", binaryCandidates.OrderCount(), binaryCandidates.OrderAt, func(op kind.BinaryOp) bool {
		return op >= kind.BinaryLess && op <= kind.BinaryGreaterEqual
	})
	if got, want := binaryCandidates.ArithmeticCount(), 7; got != want {
		t.Fatalf("ArithmeticCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.ConcatCount(), 1; got != want {
		t.Fatalf("ConcatCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.BitwiseCount(), 5; got != want {
		t.Fatalf("BitwiseCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.EqualityCount(), 2; got != want {
		t.Fatalf("EqualityCount = %d, want %d", got, want)
	}
	if got, want := binaryCandidates.OrderCount(), 4; got != want {
		t.Fatalf("OrderCount = %d, want %d", got, want)
	}

	numeric := flow.Candidates().Unary()
	if got, want := numeric.NumericCount(), 1; got != want {
		t.Fatalf("UnaryNumericCount = %d, want %d", got, want)
	}
	term, ok := numeric.NumericAt(0)
	if !ok || !flow.Executable().Contains(term) {
		t.Fatalf("UnaryNumericAt(0) = %v/%v", term, ok)
	}
	_, op, operand, unaryOK := unaries.Get(term)
	if !unaryOK || op != kind.UnaryNeg || operand == 0 {
		t.Fatalf("UnaryNumeric source = op %v operand %v ok %v", op, operand, unaryOK)
	}
}

// applicationSourceCases is the private source-evidence denominator for this
// vertical. It deliberately has no schema, claim, or production role.
