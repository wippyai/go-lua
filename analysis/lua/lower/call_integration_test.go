package lower_test

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

// CallBoundary is the sole final dynamic-call cut. It carries the ordinary
// resume or tail return and the three owning-Body exceptional outcomes; it
// replaces the retired Program-wide CallDecision and CallTailExit planes.
func TestCallBoundariesRetainExactDynamicDisposition(t *testing.T) {
	p := parseBindLower(t, `
before()
if outer then
  guarded()
end
after()
`)
	calls := p.Flow().Authored().Calls()
	boundaries := p.Flow().Causal().Boundaries()
	if calls.Count() != 3 || boundaries.Count() != 3 {
		t.Fatalf("Calls/Boundaries = %d/%d, want 3/3", calls.Count(), boundaries.Count())
	}
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		boundary, ok := boundaries.For(call)
		if !ok || boundary.Call != call || boundary.Normal == 0 || boundary.TailReturn != 0 {
			t.Fatalf("CallBoundary(%v) = %#v/%v, want ordinary resume", call, boundary, ok)
		}
		for _, exit := range []struct {
			name string
			term keyspace.Term
			kind kind.OutcomeKind
		}{
			{"throw", boundary.Throw, kind.OutcomeThrow},
			{"yield", boundary.Yield, kind.OutcomeYield},
			{"cancel", boundary.Cancel, kind.OutcomeCancel},
		} {
			outcome, outcomeOK := p.Flow().Outcomes().Get(exit.term)
			if !outcomeOK || outcome.Kind != exit.kind {
				t.Fatalf("CallBoundary(%v) %s = %v/%v, want %v Outcome", call, exit.name, exit.term, outcomeOK, exit.kind)
			}
		}
	}
}

func TestTailCallBoundaryUsesReturnOutcome(t *testing.T) {
	p := parseBindLower(t, `
local function tail()
  return tailfn()
end
local function prefix()
  return 1, prefixfn()
end
`)
	calls := p.Flow().Authored().Calls()
	if calls.Count() != 2 {
		t.Fatalf("CallCount = %d, want 2", calls.Count())
	}
	tail, _ := calls.At(0)
	prefix, _ := calls.At(1)
	tailBoundary, tailOK := p.Flow().Causal().Boundaries().For(tail)
	prefixBoundary, prefixOK := p.Flow().Causal().Boundaries().For(prefix)
	if !tailOK || tailBoundary.TailReturn == 0 || tailBoundary.Normal != 0 {
		t.Fatalf("tail CallBoundary = %#v/%v", tailBoundary, tailOK)
	}
	if outcome, ok := p.Flow().Outcomes().Get(tailBoundary.TailReturn); !ok || outcome.Kind != kind.OutcomeReturn || outcome.Target != 0 {
		t.Fatalf("tail destination = %#v/%v, want terminal Return", outcome, ok)
	}
	if !prefixOK || prefixBoundary.TailReturn != 0 || prefixBoundary.Normal == 0 {
		t.Fatalf("prefix CallBoundary = %#v/%v, want ordinary resume", prefixBoundary, prefixOK)
	}
}

// An open Return Values row may forward the current function's Vararg rather
// than a Call result.  That is a valid authored result shape, but it does not
// create a Causal tail boundary; only the two actual Call tails do.
func TestOpenVarargReturnIsNotCausalTailCall(t *testing.T) {
	p := parseBindLower(t, `
local function inner(...: number)
  return ...
end
local function fwd(...: number)
  return inner(...)
end
return fwd(1, 2, 3)
`)
	returns := p.Flow().Authored().Control().Returns()
	varargTails, callTails := 0, 0
	for index := 0; index < returns.Count(); index++ {
		returned, ok := returns.At(index)
		if !ok {
			t.Fatalf("missing Return %d", index)
		}
		returnOwner, values, ok := returns.Get(returned)
		if !ok {
			t.Fatalf("Return(%v) has no Values", returned)
		}
		tail := valuesTail(t, p, values)
		switch keyspace.TermFamily(tail) {
		case keyspace.FamilyVararg:
			varargTails++
			varargOwner, cell, varargOK := p.Flow().Authored().Storage().Varargs().Get(tail)
			if !varargOK || varargOwner != returnOwner || cell == 0 || !p.Flow().Executable().Contains(tail) {
				t.Fatalf("Return(%v) Vararg(%v) = owner %v cell %v ok %v executable %v, want owning live Vararg", returned, tail, varargOwner, cell, varargOK, p.Flow().Executable().Contains(tail))
			}
		case keyspace.FamilyCall:
			callTails++
		}
	}
	if varargTails != 1 || callTails != 2 {
		t.Fatalf("open Return tails = Vararg:%d Call:%d, want Vararg:1 Call:2", varargTails, callTails)
	}
	calls := p.Flow().Authored().Calls()
	boundaries := p.Flow().Causal().Boundaries()
	if calls.Count() != 2 || boundaries.Count() != 2 {
		t.Fatalf("Calls/Boundaries = %d/%d, want 2/2", calls.Count(), boundaries.Count())
	}
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok {
			t.Fatalf("missing Call %d", index)
		}
		boundary, ok := boundaries.For(call)
		if !ok || boundary.TailReturn == 0 || boundary.Normal != 0 {
			t.Fatalf("Call(%v) boundary = %#v/%v, want tail-only disposition", call, boundary, ok)
		}
	}
}

func TestDeadAndStaticCallsHaveNoBoundary(t *testing.T) {
	p := parseBindLower(t, `
do return end
dead()
type Snapshot = typeof(staticfn())
`)
	calls := p.Flow().Authored().Calls()
	if calls.Count() != 2 {
		t.Fatalf("CallCount = %d, want dead and static occurrences", calls.Count())
	}
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		if boundary, ok := p.Flow().Causal().Boundaries().For(call); ok || boundary.Call != 0 {
			t.Fatalf("dead/static CallBoundary(%v) = %#v/%v, want absent", call, boundary, ok)
		}
	}
}

func TestCallBoundaryQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `if condition then return target() end`)
	call, _ := p.Flow().Authored().Calls().At(0)
	if boundary, ok := p.Flow().Causal().Boundaries().For(call); !ok || boundary.TailReturn == 0 {
		t.Fatalf("tail CallBoundary = %#v/%v", boundary, ok)
	}
	allocations := testing.AllocsPerRun(1000, func() {
		_, _ = p.Flow().Causal().Boundaries().For(call)
	})
	if allocations != 0 {
		t.Fatalf("CallBoundary.For allocates %f times", allocations)
	}
}

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
	if access.GetCount() != 1 || access.SetCount() != 1 {
		t.Fatalf("Access candidates get/set = %d/%d, want 1/1", access.GetCount(), access.SetCount())
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
func applicationSourceCases() []sourceCase {
	return []sourceCase{
		{"application.case.arithmetic.add", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left + right", 2},
		{"application.case.arithmetic.sub", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left - right", 2},
		{"application.case.arithmetic.mul", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left * right", 2},
		{"application.case.arithmetic.div", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left / right", 2},
		{"application.case.arithmetic.idiv", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left // right", 2},
		{"application.case.arithmetic.mod", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left % right", 2},
		{"application.case.arithmetic.pow", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left ^ right", 2},
		{"application.case.arithmetic.band", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left & right", 2},
		{"application.case.arithmetic.bor", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left | right", 2},
		{"application.case.arithmetic.bxor", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left ~ right", 2},
		{"application.case.arithmetic.shl", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left << right", 2},
		{"application.case.arithmetic.shr", "ArithmeticOpExpr", "local left, right = 7, 3\nreturn left >> right", 2},
		{"application.case.concat", "StringConcatOpExpr", "local left, right = \"left\", \"right\"\nreturn left .. right", 2},
		{"application.case.relational.gt", "RelationalOpExpr", "local left, right = 7, 3\nreturn left > right", 2},
		{"application.case.relational.lt", "RelationalOpExpr", "local left, right = 7, 3\nreturn left < right", 2},
		{"application.case.relational.ge", "RelationalOpExpr", "local left, right = 7, 3\nreturn left >= right", 2},
		{"application.case.relational.le", "RelationalOpExpr", "local left, right = 7, 3\nreturn left <= right", 2},
		{"application.case.relational.eq", "RelationalOpExpr", "local left, right = 7, 3\nreturn left == right", 2},
		{"application.case.relational.ne", "RelationalOpExpr", "local left, right = 7, 3\nreturn left ~= right", 2},
		{"application.case.logical.and", "LogicalOpExpr", "local left, right = 7, 3\nreturn left and right", 2},
		{"application.case.logical.or", "LogicalOpExpr", "local left, right = false, 3\nreturn left or right", 2},
		{"application.case.unary.neg", "UnaryMinusOpExpr", "local value = 7\nreturn -value", 2},
		{"application.case.unary.not", "UnaryNotOpExpr", "local value = false\nreturn not value", 2},
		{"application.case.unary.len", "UnaryLenOpExpr", "local value = \"value\"\nreturn #value", 2},
		{"application.case.unary.bnot", "UnaryBNotOpExpr", "local value = 7\nreturn ~value", 2},
		{"application.case.call.plain.scalar", "FuncCallExpr", "local function f()\n  return 1, 2\nend\nreturn (f())", 4},
		{"application.case.call.plain.open", "FuncCallExpr", "local function f()\n  return 1, 2\nend\nreturn f()", 4},
		{"application.case.call.method.scalar", "FuncCallExpr", "local object = {\n  f = function(self)\n    return 1, 2\n  end,\n}\nreturn (object:f())", 6},
		{"application.case.call.method.open", "FuncCallExpr", "local object = {\n  f = function(self)\n    return 1, 2\n  end,\n}\nreturn object:f()", 6},
		{"application.case.function.inferred-returns", "FunctionExpr", "local f =\nfunction(value)\n  return value\nend\nreturn f", 2},
		{"application.case.function.declared-empty-returns", "FunctionExpr", "local f =\nfunction(): ()\n  return\nend\nreturn f", 2},
		{"application.case.function.declared-returns", "FunctionExpr", "local f =\nfunction(value: number): number\n  return value\nend\nreturn f", 2},
		{"application.case.function.declared-multiple-returns", "FunctionExpr", "local f =\nfunction(value: number): (number, string)\n  return value, \"ok\"\nend\nreturn f", 2},
		{"application.case.parameters.fixed", "ParList", "local f =\nfunction(first: number, second: string)\n  return first\nend\nreturn f", 2},
		{"application.case.parameters.vararg", "ParList", "local f =\nfunction(first, ...)\n  return first\nend\nreturn f", 2},
		{"application.case.parameters.typed-vararg", "ParList", "local f =\nfunction(first: number, ...: string)\n  return first\nend\nreturn f", 2},
		{"application.case.function-name.path", "FuncName", "local root = {}\nfunction root.branch(value)\n  return value\nend\nreturn root.branch", 2},
		{"application.case.function-name.method", "FuncName", "local root = {}\nfunction root:method(value)\n  return value\nend\nreturn root.method", 2},
	}
}

// TestApplicationSourceCasesHaveExactProgramWitnesses is the atomic source
// witness for the application vertical.  It deliberately starts at the
// parsed node named by each atomic source case, then follows the matching typed
// Program relation.  It does not infer a result from Case/Disposition prose,
// nor does it use family-wide counts as coverage.
func TestApplicationSourceCasesHaveExactProgramWitnesses(t *testing.T) {
	for _, sourceCase := range applicationSourceCases() {
		t.Run(string(sourceCase.ID), func(t *testing.T) {
			stmts, err := parse.ParseString(sourceCase.Source, "fixture.lua")
			if err != nil {
				t.Fatal(err)
			}
			anchor := applicationAnchor(t, stmts, sourceCase)
			if anchor.Form != sourceCase.Form || anchor.Line != sourceCase.Line || anchor.Span.StartLine == 0 || anchor.Span.File != "fixture.lua" {
				t.Fatalf("parsed application anchor = %#v for %s/%d", anchor, sourceCase.Form, sourceCase.Line)
			}
			binding := bind.BindChunk(stmts)
			p := parseBindLower(t, sourceCase.Source)
			switch node := anchor.Node.(type) {
			case *ast.ArithmeticOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.StringConcatOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, "..", node.Lhs, node.Rhs)
			case *ast.RelationalOpExpr:
				term := applicationBinaryAt(t, p, node)
				applicationBinary(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.LogicalOpExpr:
				term := applicationSelectAt(t, p, node)
				applicationSelect(t, p, term, node.Operator, node.Lhs, node.Rhs)
			case *ast.UnaryMinusOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryNeg, node.Expr)
			case *ast.UnaryNotOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryNot, node.Expr)
			case *ast.UnaryLenOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryLen, node.Expr)
			case *ast.UnaryBNotOpExpr:
				applicationUnary(t, p, applicationUnaryAt(t, p, node), kind.UnaryBitNot, node.Expr)
			case *ast.FuncCallExpr:
				applicationCall(t, p, applicationCallAt(t, p, node), node, stmts)
			case *ast.FunctionExpr:
				term := applicationFunctionAt(t, p, node)
				applicationFunction(t, p, term, node, binding)
				applicationBoundFunction(t, binding, stmts, node)
			case *ast.ParList:
				fn := applicationFunctionForParList(t, stmts, node)
				term := applicationFunctionAt(t, p, fn)
				applicationFunction(t, p, term, fn, binding)
				applicationBoundFunction(t, binding, stmts, fn)
			case *ast.FuncName:
				fn := applicationFunctionForName(t, stmts, node)
				term := applicationFunctionAt(t, p, fn)
				applicationFunction(t, p, term, fn, binding)
				applicationBoundFunction(t, binding, stmts, fn)
			default:
				t.Fatalf("unhandled application anchor %T", anchor)
			}
		})
	}
}

func applicationUnary(t *testing.T, p *program.Program, term keyspace.Term, want kind.UnaryOp, source ast.Expr) {
	t.Helper()
	flow := p.Flow()
	owner, op, operand, ok := flow.Authored().Operators().Unaries().Get(term)
	if !ok || owner == 0 || op != want || operand == 0 {
		t.Fatalf("Unary = owner %v op %v operand %v ok %v, want %v", owner, op, operand, ok, want)
	}
	applicationSameSpan(t, p, operand, source)
	if entry, ok := flow.Ports().Entry(operand); !ok || entry == 0 {
		t.Fatalf("Unary(%v) has no operand entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Unary(%v) has no normal successor", term)
	}
}

func applicationBinary(t *testing.T, p *program.Program, term keyspace.Term, operator string, lhs, rhs ast.Expr) {
	t.Helper()
	want, ok := applicationBinaryOp(operator)
	if !ok {
		t.Fatalf("unrecognized parsed binary operator %q", operator)
	}
	flow := p.Flow()
	owner, got, left, right, ok := flow.Authored().Operators().Binaries().Get(term)
	if !ok || owner == 0 || got != want || left == 0 || right == 0 {
		t.Fatalf("Binary = owner %v op %v left %v right %v ok %v, want %v", owner, got, left, right, ok, want)
	}
	applicationSameSpan(t, p, left, lhs)
	applicationSameSpan(t, p, right, rhs)
	if entry, ok := flow.Ports().Entry(left); !ok || entry == 0 {
		t.Fatalf("Binary(%v) has no left entry", term)
	}
	if entry, ok := flow.Ports().Entry(right); !ok || entry == 0 {
		t.Fatalf("Binary(%v) has no right entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Binary(%v) has no normal successor", term)
	}
}

func applicationSelect(t *testing.T, p *program.Program, term keyspace.Term, operator string, lhs, rhs ast.Expr) {
	t.Helper()
	want := kind.SelectAnd
	if operator == "or" {
		want = kind.SelectOr
	} else if operator != "and" {
		t.Fatalf("unrecognized parsed logical operator %q", operator)
	}
	flow := p.Flow()
	owner, got, left, right, ok := flow.Authored().Operators().Selects().Get(term)
	if !ok || owner == 0 || got != want || left == 0 || right == 0 {
		t.Fatalf("Select = owner %v op %v left %v right %v ok %v, want %v", owner, got, left, right, ok, want)
	}
	applicationSameSpan(t, p, left, lhs)
	applicationSameSpan(t, p, right, rhs)
	if entry, ok := flow.Ports().Entry(left); !ok || entry == 0 {
		t.Fatalf("Select(%v) has no left entry", term)
	}
	rightEntry, rightOK := flow.Ports().Entry(right)
	if !rightOK || rightEntry == 0 {
		t.Fatalf("Select(%v) has no right entry", term)
	}
	guardedRight := false
	for index := 0; index < flow.Causal().Edges().Count(); index++ {
		edge, edgeOK := flow.Causal().Edges().At(index)
		if edgeOK && edge.Decision == term && edge.Truth == (operator == "and") && edge.To == rightEntry {
			guardedRight = true
		}
	}
	if !guardedRight {
		t.Fatalf("Select(%v) has no guarded right entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Select(%v) has no normal successor", term)
	}
}

func applicationCall(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FuncCallExpr, stmts []ast.Stmt) {
	t.Helper()
	flow := p.Flow()
	owner, callee, receiver, actuals, ok := flow.Authored().Calls().Get(term)
	if !ok || owner == 0 || callee == 0 || actuals == 0 {
		t.Fatalf("Call = owner %v callee %v receiver %v actuals %v ok %v", owner, callee, receiver, actuals, ok)
	}
	direct, _ := flow.DirectFunctions().Call(term)
	if (node.Receiver != nil) != (receiver != 0) {
		t.Fatalf("Call receiver = %v for parsed receiver %T", receiver, node.Receiver)
	}
	if fixed, ok := flow.Authored().Values().Len(actuals); !ok || fixed != len(node.Args) {
		t.Fatalf("Call actual fixed count = %d/%v, want parsed %d", fixed, ok, len(node.Args))
	}
	if node.Receiver != nil {
		// A Lua table member is mutable.  Even this literal-table method call
		// has no flow proof that the member remains that closure at the call.
		if direct != 0 {
			t.Fatalf("mutable method Call direct candidate = %v, want absent", direct)
		}
	} else {
		function := applicationOnlyFunctionExpr(t, stmts)
		wantDirect := applicationFunctionAt(t, p, function)
		if direct != wantDirect {
			t.Fatalf("plain local Call direct candidate = %v, want source Function %v", direct, wantDirect)
		}
	}
	if types, ok := p.Static().Contracts().Calls().TypeArgumentCount(term); !ok || types != len(node.TypeArgs) {
		t.Fatalf("Call type-argument count = %d/%v, want parsed %d", types, ok, len(node.TypeArgs))
	}
	if entry, ok := flow.Ports().Entry(callee); !ok || entry == 0 {
		t.Fatalf("Call(%v) has no callee entry", term)
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Call(%v) has no normal successor", term)
	}
	// The atomic call sources return their anchor directly.  Parentheses are
	// already reflected by the parser's AdjustRet bit, so this proves the
	// Program's fixed-versus-final-open result relation from syntax rather than
	// from a prose label.
	returned := returnOwnedBy(t, p, owner)
	_, values, ok := flow.Authored().Control().Returns().Get(returned)
	if !ok {
		t.Fatalf("Call(%v) owner has no Return Values", term)
	}
	fixed, fixedOK := flow.Authored().Values().Len(values)
	tail := valuesTail(t, p, values)
	if node.AdjustRet {
		if !fixedOK || fixed != 1 || valueAt(t, p, values, 0) != term || tail != 0 {
			t.Fatalf("scalar Call result Values = fixed %d/%v value %v tail %v", fixed, fixedOK, valueAt(t, p, values, 0), tail)
		}
	} else if !fixedOK || fixed != 0 || tail != term {
		t.Fatalf("open Call result Values = fixed %d/%v tail %v, want authored Call %v", fixed, fixedOK, tail, term)
	}
	boundary, boundaryOK := flow.Causal().Boundaries().For(term)
	if !boundaryOK || boundary.Throw == 0 || boundary.Yield == 0 || boundary.Cancel == 0 {
		t.Fatal("Call lacks a shared non-normal outcome")
	}
}

func applicationOnlyFunctionExpr(t *testing.T, stmts []ast.Stmt) *ast.FunctionExpr {
	t.Helper()
	var functions []*ast.FunctionExpr
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if function, ok := node.(*ast.FunctionExpr); ok {
			functions = append(functions, function)
		}
	})
	if len(functions) != 1 {
		t.Fatalf("plain call source Functions = %d, want exactly one", len(functions))
	}
	return functions[0]
}

func applicationFunction(t *testing.T, p *program.Program, term keyspace.Term, node *ast.FunctionExpr, binding *bind.Result) {
	t.Helper()
	flow := p.Flow()
	owner, body, vararg, ok := flow.Authored().Functions().Get(term)
	if !ok || owner == 0 || body == 0 {
		t.Fatalf("Function = owner %v body %v vararg %v ok %v", owner, body, vararg, ok)
	}
	slots := binding.ParamSlots(node)
	wantFormals := len(slots)
	if node.ParList != nil && node.ParList.HasVargs {
		wantFormals--
	}
	if got, ok := p.Source().Formals().Len(term); !ok || got != wantFormals || (node.ParList != nil && node.ParList.HasVargs) != (vararg != 0) {
		t.Fatalf("Function formals/vararg = %d/%v/%v, binder non-vararg slots %d parsed-vararg %v", got, ok, vararg, wantFormals, node.ParList != nil && node.ParList.HasVargs)
	}
	if known, ok := p.Static().Contracts().Functions().Get(term); !ok || known != node.ReturnsKnown {
		t.Fatalf("Function returns-known = %v/%v, want parsed %v", known, ok, node.ReturnsKnown)
	}
	if count, ok := p.Static().Contracts().Functions().ReturnCount(term); !ok || count != len(node.ReturnTypes) {
		t.Fatalf("Function return count = %d/%v, want parsed %d", count, ok, len(node.ReturnTypes))
	}
	if next, ok := flow.Ports().Finish(term); !ok || next == 0 {
		t.Fatalf("Function(%v) has no normal successor", term)
	}
}

func applicationBoundFunction(t *testing.T, result *bind.Result, stmts []ast.Stmt, node *ast.FunctionExpr) {
	t.Helper()
	origin, ok := result.FunctionOrigin(node)
	if !ok || origin.Func != node {
		t.Fatal("binder lost exact FunctionExpr origin")
	}
	if want := applicationFunctionOrigin(stmts, node); origin.Kind != want {
		t.Fatalf("binder function origin = %v, want parsed origin %v", origin.Kind, want)
	}
	if node.ParList == nil {
		return
	}
	slots := result.ParamSlots(node)
	implicitSelf := origin.Kind == bind.FunctionOriginMethod && (len(node.ParList.Names) == 0 || node.ParList.Names[0] != "self")
	want := len(node.ParList.Names)
	if implicitSelf {
		want++
	}
	if node.ParList.HasVargs {
		want++
	}
	if len(slots) != want {
		t.Fatalf("binder parameter slots = %d, want exact authored layout %d", len(slots), want)
	}
	offset := 0
	if implicitSelf {
		first := slots[0]
		if first.Name != "self" || !first.ImplicitSelf || first.Vararg || first.SourceIndex != -1 {
			t.Fatalf("binder implicit receiver = %#v", first)
		}
		offset = 1
	}
	for index, name := range node.ParList.Names {
		slot := slots[offset+index]
		if slot.Name != name || slot.ImplicitSelf || slot.Vararg || slot.SourceIndex != index || slot.Position != applicationNamePosition(node.ParList, index) || slot.Type != applicationNameType(node.ParList, index) {
			t.Fatalf("binder formal %d = %#v, want parsed name/type/position", index, slot)
		}
	}
	if node.ParList.HasVargs {
		slot := slots[len(slots)-1]
		if slot.Name != "..." || slot.ImplicitSelf || !slot.Vararg || slot.SourceIndex != len(node.ParList.Names) || slot.Position != node.ParList.VarargPosition || slot.Type != node.ParList.VarargType {
			t.Fatalf("binder vararg = %#v, want parsed vararg", slot)
		}
	}
}

func applicationNamePosition(list *ast.ParList, index int) ast.Position {
	if list == nil || index < 0 || index >= len(list.NamePositions) {
		return ast.Position{}
	}
	return list.NamePositions[index]
}

func applicationNameType(list *ast.ParList, index int) ast.TypeExpr {
	if list == nil || index < 0 || index >= len(list.Types) {
		return nil
	}
	return list.Types[index]
}

func applicationFunctionOrigin(stmts []ast.Stmt, function *ast.FunctionExpr) bind.FunctionOriginKind {
	for _, stmt := range stmts {
		switch node := stmt.(type) {
		case *ast.FuncDefStmt:
			if node.Func == function {
				if node.Name != nil && node.Name.Receiver != nil {
					return bind.FunctionOriginMethod
				}
				return bind.FunctionOriginDeclaration
			}
		case *ast.LocalAssignStmt:
			for _, expr := range node.Exprs {
				if expr == function {
					return bind.FunctionOriginLocalAssignment
				}
			}
		}
	}
	return bind.FunctionOriginLiteral
}

func applicationBinaryOp(operator string) (kind.BinaryOp, bool) {
	switch operator {
	case "+":
		return kind.BinaryAdd, true
	case "-":
		return kind.BinarySub, true
	case "*":
		return kind.BinaryMul, true
	case "/":
		return kind.BinaryDiv, true
	case "//":
		return kind.BinaryIDiv, true
	case "%":
		return kind.BinaryMod, true
	case "^":
		return kind.BinaryPow, true
	case "..":
		return kind.BinaryConcat, true
	case "&":
		return kind.BinaryBitAnd, true
	case "|":
		return kind.BinaryBitOr, true
	case "~":
		return kind.BinaryBitXor, true
	case "<<":
		return kind.BinaryShiftLeft, true
	case ">>":
		return kind.BinaryShiftRight, true
	case "==":
		return kind.BinaryEqual, true
	case "~=":
		return kind.BinaryNotEqual, true
	case "<":
		return kind.BinaryLess, true
	case "<=":
		return kind.BinaryLessEqual, true
	case ">":
		return kind.BinaryGreater, true
	case ">=":
		return kind.BinaryGreaterEqual, true
	}
	return 0, false
}

func applicationUnaryAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	unaries := p.Flow().Authored().Operators().Unaries()
	return applicationTermAt(t, p, node, unaries.Count, unaries.At, "Unary")
}
func applicationBinaryAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	binaries := p.Flow().Authored().Operators().Binaries()
	return applicationTermAt(t, p, node, binaries.Count, binaries.At, "Binary")
}
func applicationSelectAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	selects := p.Flow().Authored().Operators().Selects()
	return applicationTermAt(t, p, node, selects.Count, selects.At, "Select")
}
func applicationCallAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	return applicationTermAt(t, p, node, calls.Count, calls.At, "Call")
}
func applicationFunctionAt(t *testing.T, p *program.Program, node ast.PositionHolder) keyspace.Term {
	t.Helper()
	functions := p.Flow().Authored().Functions()
	return applicationTermAt(t, p, node, functions.Count, functions.At, "Function")
}

func applicationTermAt(t *testing.T, p *program.Program, node ast.PositionHolder, count func() int, at func(int) (keyspace.Term, bool), family string) keyspace.Term {
	t.Helper()
	var found keyspace.Term
	for index := 0; index < count(); index++ {
		term, ok := at(index)
		if !ok {
			t.Fatalf("%sAt(%d) missing", family, index)
		}
		span, ok := p.Source().Identity().Span(term)
		if !ok {
			t.Fatalf("%s %v has no span", family, term)
		}
		if span != applicationASTSpan(node) {
			continue
		}
		if found != 0 {
			t.Fatalf("%s anchor %d:%d is ambiguous", family, node.Line(), node.Column())
		}
		found = term
	}
	if found == 0 {
		t.Fatalf("no %s has exact parsed span %d:%d-%d:%d", family, node.Line(), node.Column(), node.LastLine(), node.LastColumn())
	}
	return found
}

func applicationSameSpan(t *testing.T, p *program.Program, term keyspace.Term, source ast.PositionHolder) {
	t.Helper()
	got, ok := p.Source().Identity().Span(term)
	want := applicationASTSpan(source)
	if !ok || got != want {
		t.Fatalf("Program term %v span = %#v/%v, want parsed child %#v", term, got, ok, want)
	}
}

type applicationSourceAnchor struct {
	Form string
	Line int
	Span source.Span
	Node any
}

func applicationAnchor(t *testing.T, stmts []ast.Stmt, sourceCase sourceCase) applicationSourceAnchor {
	t.Helper()
	if sourceCase.Form == "ParList" {
		var lists []*ast.ParList
		applicationWalk(stmts, func(node ast.PositionHolder) {
			if fn, ok := node.(*ast.FunctionExpr); ok && fn.Line() == sourceCase.Line && fn.ParList != nil {
				lists = append(lists, fn.ParList)
			}
		})
		if len(lists) != 1 {
			t.Fatalf("parsed ParList anchors at line %d = %d, want exactly one", sourceCase.Line, len(lists))
		}
		return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationParListSpan(lists[0]), Node: lists[0]}
	}
	if sourceCase.Form == "FuncName" {
		var names []*ast.FuncName
		for _, stmt := range stmts {
			if def, ok := stmt.(*ast.FuncDefStmt); ok && def.Line() == sourceCase.Line && def.Name != nil {
				names = append(names, def.Name)
			}
		}
		if len(names) != 1 {
			t.Fatalf("parsed FuncName anchors at line %d = %d, want exactly one", sourceCase.Line, len(names))
		}
		return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationFuncNameSpan(names[0]), Node: names[0]}
	}
	var found []ast.PositionHolder
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if node.Line() == sourceCase.Line && applicationForm(node) == sourceCase.Form {
			found = append(found, node)
		}
	})
	if len(found) != 1 {
		t.Fatalf("parsed %s anchors at line %d = %d, want exactly one", sourceCase.Form, sourceCase.Line, len(found))
	}
	return applicationSourceAnchor{Form: sourceCase.Form, Line: sourceCase.Line, Span: applicationASTSpan(found[0]), Node: found[0]}
}

func applicationASTSpan(node ast.PositionHolder) source.Span {
	return source.Span{File: "fixture.lua", StartLine: uint32(node.Line()), StartCol: uint32(node.Column()), EndLine: uint32(node.LastLine()), EndCol: uint32(node.LastColumn())}
}

func applicationParListSpan(list *ast.ParList) source.Span {
	if list == nil {
		return source.Span{}
	}
	if len(list.NamePositions) != 0 {
		pos := list.NamePositions[0]
		return source.Span{File: "fixture.lua", StartLine: uint32(pos.Line), StartCol: uint32(pos.Column), EndLine: uint32(pos.EndLine), EndCol: uint32(pos.EndColumn)}
	}
	if list.HasVargs {
		pos := list.VarargPosition
		return source.Span{File: "fixture.lua", StartLine: uint32(pos.Line), StartCol: uint32(pos.Column), EndLine: uint32(pos.EndLine), EndCol: uint32(pos.EndColumn)}
	}
	return source.Span{}
}

func applicationFuncNameSpan(name *ast.FuncName) source.Span {
	if name == nil {
		return source.Span{}
	}
	if name.Func != nil {
		return applicationASTSpan(name.Func)
	}
	if name.Receiver != nil {
		return applicationASTSpan(name.Receiver)
	}
	return source.Span{}
}

func applicationFunctionForParList(t *testing.T, stmts []ast.Stmt, list *ast.ParList) *ast.FunctionExpr {
	t.Helper()
	var found *ast.FunctionExpr
	applicationWalk(stmts, func(node ast.PositionHolder) {
		if fn, ok := node.(*ast.FunctionExpr); ok && fn.ParList == list {
			found = fn
		}
	})
	if found == nil {
		t.Fatal("ParList has no owning FunctionExpr")
	}
	return found
}
func applicationFunctionForName(t *testing.T, stmts []ast.Stmt, name *ast.FuncName) *ast.FunctionExpr {
	t.Helper()
	var found *ast.FunctionExpr
	for _, stmt := range stmts {
		if def, ok := stmt.(*ast.FuncDefStmt); ok && def.Name == name {
			found = def.Func
		}
	}
	if found == nil {
		t.Fatal("FuncName has no owning FuncDefStmt")
	}
	return found
}

func applicationForm(node any) string {
	switch node.(type) {
	case *ast.ArithmeticOpExpr:
		return "ArithmeticOpExpr"
	case *ast.StringConcatOpExpr:
		return "StringConcatOpExpr"
	case *ast.RelationalOpExpr:
		return "RelationalOpExpr"
	case *ast.LogicalOpExpr:
		return "LogicalOpExpr"
	case *ast.UnaryMinusOpExpr:
		return "UnaryMinusOpExpr"
	case *ast.UnaryNotOpExpr:
		return "UnaryNotOpExpr"
	case *ast.UnaryLenOpExpr:
		return "UnaryLenOpExpr"
	case *ast.UnaryBNotOpExpr:
		return "UnaryBNotOpExpr"
	case *ast.FuncCallExpr:
		return "FuncCallExpr"
	case *ast.FunctionExpr:
		return "FunctionExpr"
	case *ast.ParList:
		return "ParList"
	case *ast.FuncName:
		return "FuncName"
	}
	return fmt.Sprintf("%T", node)
}

// applicationWalk is intentionally a closed walk over the source forms that
// can contain an application expression.  It is test-side syntax traversal,
// not reflection or a production lowering visitor.
func applicationWalk(stmts []ast.Stmt, visit func(ast.PositionHolder)) {
	var stmt func(ast.Stmt)
	var expr func(ast.Expr)
	stmt = func(current ast.Stmt) {
		if current == nil {
			return
		}
		switch node := current.(type) {
		case *ast.AssignStmt:
			for _, item := range node.Lhs {
				expr(item)
			}
			for _, item := range node.Rhs {
				expr(item)
			}
		case *ast.LocalAssignStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
		case *ast.FuncCallStmt:
			expr(node.Expr)
		case *ast.DoBlockStmt:
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.WhileStmt:
			expr(node.Condition)
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.RepeatStmt:
			for _, child := range node.Stmts {
				stmt(child)
			}
			expr(node.Condition)
		case *ast.IfStmt:
			expr(node.Condition)
			for _, child := range node.Then {
				stmt(child)
			}
			for _, child := range node.Else {
				stmt(child)
			}
		case *ast.NumberForStmt:
			expr(node.Init)
			expr(node.Limit)
			expr(node.Step)
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.GenericForStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
			for _, child := range node.Stmts {
				stmt(child)
			}
		case *ast.FuncDefStmt:
			expr(node.Func)
		case *ast.ReturnStmt:
			for _, item := range node.Exprs {
				expr(item)
			}
		}
	}
	expr = func(current ast.Expr) {
		if current == nil {
			return
		}
		switch node := current.(type) {
		case *ast.AttrGetExpr:
			expr(node.Object)
			expr(node.Key)
		case *ast.TableExpr:
			for _, field := range node.Fields {
				if field != nil {
					expr(field.Key)
					expr(field.Value)
				}
			}
		case *ast.FuncCallExpr:
			visit(node)
			expr(node.Func)
			expr(node.Receiver)
			for _, item := range node.Args {
				expr(item)
			}
		case *ast.LogicalOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.RelationalOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.StringConcatOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.ArithmeticOpExpr:
			visit(node)
			expr(node.Lhs)
			expr(node.Rhs)
		case *ast.UnaryMinusOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryNotOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryLenOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.UnaryBNotOpExpr:
			visit(node)
			expr(node.Expr)
		case *ast.FunctionExpr:
			visit(node)
			for _, child := range node.Stmts {
				stmt(child)
			}
		}
	}
	for _, current := range stmts {
		stmt(current)
	}
}

func applicationOutcome(
	t *testing.T,
	p *program.Program,
	exit, body keyspace.Term,
	kind flowkind.OutcomeKind,
	next keyspace.Term,
) {
	t.Helper()
	got, ok := p.Flow().Outcomes().Get(exit)
	gotBody, gotKind, target := got.Body, got.Kind, got.Target
	if !ok || gotBody != body || gotKind != kind || target != 0 {
		t.Fatalf("Outcome(%v) = %v/%v/%v/%v, want %v/%v/0/true", exit, gotBody, gotKind, target, ok, body, kind)
	}
	gotNext, nextOK := p.Flow().Outcomes().Propagation(exit)
	if next == 0 {
		if nextOK {
			t.Fatalf("OutcomeSuccessor(%v) = %v/%v, want terminal", exit, gotNext, nextOK)
		}
		return
	}
	if !nextOK || gotNext != next {
		t.Fatalf("OutcomeSuccessor(%v) = %v/%v, want %v", exit, gotNext, nextOK, next)
	}
}
