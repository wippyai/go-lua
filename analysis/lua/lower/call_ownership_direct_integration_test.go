package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/static"
)

var (
	executableSink     bool
	directFunctionSink keyspace.Term
	directFunctionOK   bool
)

func callAt(t *testing.T, p *program.Program, index int) (keyspace.Term, keyspace.Term, keyspace.Term) {
	t.Helper()
	calls := p.Flow().Authored().Calls()
	call, ok := calls.At(index)
	if !ok {
		t.Fatalf("CallAt(%d) is absent", index)
	}
	_, callee, _, actuals, ok := calls.Get(call)
	if !ok {
		t.Fatalf("Call(%v) is malformed", call)
	}
	return call, callee, actuals
}

func actualAt(t *testing.T, p *program.Program, callIndex, actualIndex int) keyspace.Term {
	t.Helper()
	_, _, actuals := callAt(t, p, callIndex)
	actual, ok := p.Flow().Authored().Values().Member(actuals, actualIndex)
	if !ok {
		t.Fatalf("CallAt(%d) actual %d is absent", callIndex, actualIndex)
	}
	return actual
}

func unaryCallActual(t *testing.T, p *program.Program) keyspace.Term {
	t.Helper()
	authored := p.Flow().Authored()
	calls := authored.Calls()
	values := authored.Values()
	var found keyspace.Term
	for index := 0; index < calls.Count(); index++ {
		_, _, actuals := callAt(t, p, index)
		if length, ok := values.Len(actuals); !ok || length != 1 {
			continue
		}
		actual, ok := values.Member(actuals, 0)
		if !ok || found != 0 {
			t.Fatal("source does not have exactly one unary Call")
		}
		found = actual
	}
	if found == 0 {
		t.Fatal("source has no unary Call")
	}
	return found
}

func TestExecutableDistinguishesSourceReachabilityFromDynamicDispatch(t *testing.T) {
	for _, test := range []struct {
		name, source string
		want         bool
	}{
		{"live dynamic", `dynamic()`, true},
		{"live external", `require("module")`, true},
		{"dead dynamic", `do return end; dynamic()`, false},
		{"dead external", `do return end; require("module")`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			call, callee, _ := callAt(t, p, 0)
			flow := p.Flow()
			if got := flow.Executable().Contains(call); got != test.want {
				t.Fatalf("Executable(%v) = %v, want %v", call, got, test.want)
			}
			if function, ok := flow.DirectFunctions().For(callee); ok || function != 0 {
				t.Fatalf("dynamic callee DirectFunction = %v/%v", function, ok)
			}
		})
	}
	p := parseBindLower(t, `dynamic()`)
	flow := p.Flow()
	if flow.Executable().Contains(0) {
		t.Fatal("zero Term became executable")
	}
	entry, ok := p.Source().Index().Entry()
	if !ok || !flow.Executable().Contains(entry) {
		t.Fatal("Entry Body lost its executable proof")
	}
}

func TestDirectFunctionUsesOnlyFunctionOccurrencesAndSealProvedLiveReads(t *testing.T) {
	t.Run("literal", func(t *testing.T) {
		p := parseBindLower(t, `callback(function() end)`)
		flow := p.Flow()
		function, _ := flow.Authored().Functions().At(0)
		actual := actualAt(t, p, 0, 0)
		if actual != function {
			t.Fatalf("literal actual = %v, want Function %v", actual, function)
		}
		if got, ok := flow.DirectFunctions().For(actual); !ok || got != function {
			t.Fatalf("literal DirectFunction = %v/%v, want %v", got, ok, function)
		}
	})
	t.Run("dead literal stays exact without making its Call live", func(t *testing.T) {
		p := parseBindLower(t, `do return end; callback(function() end)`)
		flow := p.Flow()
		call, _ := flow.Authored().Calls().At(0)
		function, _ := flow.Authored().Functions().At(0)
		actual := actualAt(t, p, 0, 0)
		if flow.Executable().Contains(call) {
			t.Fatal("dead literal Call became executable")
		}
		if got, ok := flow.DirectFunctions().For(actual); !ok || got != function {
			t.Fatalf("dead literal DirectFunction = %v/%v, want %v", got, ok, function)
		}
	})

	for _, test := range []struct {
		name, source  string
		want          bool
		functionIndex int
	}{
		{
			name: "dominated assignment",
			source: `local f
f = function() end
callback(f)`,
			want: true,
		},
		{
			name: "branch does not dominate",
			source: `local f
if condition then f = function() end end
callback(f)`,
		},
		{
			name: "reassignment invalidates singleton",
			source: `local f = function() end
f = other
callback(f)`,
		},
		{
			name: "same activation captured installation",
			source: `local f
local run = function()
  f = function() end
  callback(f)
end
run()`,
			want:          true,
			functionIndex: 1,
		},
		{
			name: "cross activation capture",
			source: `local f
local run = function() callback(f) end
f = function() end
run()`,
		},
		{
			name: "dead read",
			source: `local f = function() end
do return end
callback(f)`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			flow := p.Flow()
			function, _ := flow.Authored().Functions().At(test.functionIndex)
			actual := unaryCallActual(t, p)
			got, ok := flow.DirectFunctions().For(actual)
			if test.want {
				if !ok || got != function {
					t.Fatalf("DirectFunction(%v) = %v/%v, want %v", actual, got, ok, function)
				}
				return
			}
			if ok || got != 0 {
				t.Fatalf("DirectFunction(%v) = %v/%v, want unproved", actual, got, ok)
			}
		})
	}
}

func TestDirectFunctionPreservesCallbackActualUseShape(t *testing.T) {
	p := parseBindLower(t, `
local callback = function() end
receiver:register(callback)
`)
	flow := p.Flow()
	call, _, actuals := callAt(t, p, 0)
	_, _, receiver, gotActuals, ok := flow.Authored().Calls().Get(call)
	if !ok || receiver == 0 || gotActuals != actuals {
		t.Fatalf("method Call shape = receiver %v actuals %v/%v", receiver, gotActuals, ok)
	}
	if length, ok := flow.Authored().Values().Len(actuals); !ok || length != 1 {
		t.Fatalf("method actual width = %d/%v, want 1", length, ok)
	}
	actual := actualAt(t, p, 0, 0)
	function, _ := flow.Authored().Functions().At(0)
	if direct, ok := flow.DirectFunctions().For(actual); !ok || direct != function {
		t.Fatalf("callback actual DirectFunction = %v/%v, want %v", direct, ok, function)
	}
	if direct, ok := flow.DirectFunctions().For(receiver); ok || direct != 0 {
		t.Fatalf("receiver DirectFunction = %v/%v, want no callback proof", direct, ok)
	}
}

func TestCallAuthorityIsAlphaStable(t *testing.T) {
	left := lowerNamed(t, "call_alpha.lua", `local callback
callback = function() end
consume(callback)`)
	right := lowerNamed(t, "call_alpha.lua", `local handler
handler = function() end
consume(handler)`)
	leftFlow, rightFlow := left.Flow(), right.Flow()
	leftAuthored, rightAuthored := leftFlow.Authored(), rightFlow.Authored()
	leftCalls, rightCalls := leftAuthored.Calls(), rightAuthored.Calls()
	leftReads, rightReads := leftAuthored.Storage().Reads(), rightAuthored.Storage().Reads()
	if leftCalls.Count() != rightCalls.Count() || leftReads.Count() != rightReads.Count() {
		t.Fatal("alpha-equivalent call authority changed family counts")
	}
	for index := 0; index < leftCalls.Count(); index++ {
		leftCall, _ := leftCalls.At(index)
		rightCall, _ := rightCalls.At(index)
		if leftFlow.Executable().Contains(leftCall) != rightFlow.Executable().Contains(rightCall) {
			t.Fatalf("alpha Executable[%d] differs", index)
		}
	}
	for index := 0; index < leftReads.Count(); index++ {
		leftRead, _ := leftReads.At(index)
		rightRead, _ := rightReads.At(index)
		leftFunction, leftOK := leftFlow.DirectFunctions().Read(leftRead)
		rightFunction, rightOK := rightFlow.DirectFunctions().Read(rightRead)
		if leftFunction != rightFunction || leftOK != rightOK {
			t.Fatalf("alpha DirectFunction Read[%d] = %v/%v and %v/%v", index, leftFunction, leftOK, rightFunction, rightOK)
		}
	}
}

func TestCallAuthorityQueriesDoNotAllocate(t *testing.T) {
	p := parseBindLower(t, `local callback = function() end; consume(callback)`)
	flow := p.Flow()
	call, _ := flow.Authored().Calls().At(0)
	actual := actualAt(t, p, 0, 0)
	allocations := testing.AllocsPerRun(1000, func() {
		executableSink = flow.Executable().Contains(call)
		directFunctionSink, directFunctionOK = flow.DirectFunctions().For(actual)
	})
	if allocations != 0 {
		t.Fatalf("call authority queries allocate %f times", allocations)
	}
}

func TestSourceCallVerticalKeepsRuntimeAndStaticInputsDisjoint(t *testing.T) {
	p := parseBindLower(t, `local function apply<T>(value: T): T
  return value
end
local receiver = { apply = apply }
return apply::<string>(1), receiver:apply::<integer>(2)`)
	flow := p.Flow()
	calls := flow.Authored().Calls()
	values := flow.Authored().Values()
	staticView := p.Static()
	if calls.Count() != 2 {
		t.Fatalf("CallCount = %d, want 2", calls.Count())
	}
	for index, want := range []struct {
		primitive static.PrimitiveKind
		method    bool
	}{
		{primitive: static.PrimitiveString},
		{primitive: static.PrimitiveInteger, method: true},
	} {
		call, ok := calls.At(index)
		if !ok {
			t.Fatalf("missing Call %d", index)
		}
		_, callee, receiver, actuals, ok := calls.Get(call)
		if !ok || callee == 0 || actuals == 0 || (receiver != 0) != want.method {
			t.Fatalf("Call %d = callee %v receiver %v actuals %v ok %v", index, callee, receiver, actuals, ok)
		}
		if count, ok := values.Len(actuals); !ok || count != 1 {
			t.Fatalf("Call %d runtime actual count = %d/%v, want 1", index, count, ok)
		}
		if _, tail, ok := values.Get(actuals); !ok || tail != 0 {
			t.Fatalf("Call %d runtime actual tail = %v/%v, want closed", index, tail, ok)
		}
		if count, ok := staticView.Contracts().Calls().TypeArgumentCount(call); !ok || count != 1 {
			t.Fatalf("Call %d static argument count = %d/%v, want 1", index, count, ok)
		}
		argument, ok := staticView.Contracts().Calls().TypeArgumentAt(call, 0)
		if !ok {
			t.Fatalf("Call %d static argument = %v/%v", index, argument, ok)
		}
		if primitive, ok := staticView.Types().Primitives().Get(argument); !ok || primitive != want.primitive {
			t.Fatalf("Call %d primitive = %v/%v, want %v", index, primitive, ok, want.primitive)
		}
	}
}

func TestSourceCallVerticalPreservesOpenFinalArgument(t *testing.T) {
	p := parseBindLower(t, `local function source()
  return 1, 2
end
local function sink(...)
  return ...
end
return sink(0, source())`)
	flow := p.Flow()
	if flow.Authored().Calls().Count() != 2 {
		t.Fatalf("CallCount = %d, want source and sink", flow.Authored().Calls().Count())
	}
	sink, _ := flow.Authored().Calls().At(1)
	_, _, _, actuals, ok := flow.Authored().Calls().Get(sink)
	if !ok {
		t.Fatal("sink Call missing")
	}
	if count, ok := flow.Authored().Values().Len(actuals); !ok || count != 1 {
		t.Fatalf("sink fixed actuals = %d/%v, want 1", count, ok)
	}
	if tail := valuesTail(t, p, actuals); tail == 0 {
		t.Fatal("final source() result was flattened instead of retained as open Values")
	}
}
