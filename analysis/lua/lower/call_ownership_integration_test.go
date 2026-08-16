package lower_test

import (
	"strconv"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program"
	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
	"github.com/wippyai/go-lua/analysis/program/static"
)

func TestSourceApplicationSharedBodyOutcomesAndCallExits(t *testing.T) {
	p := parseBindLower(t, `
do
  do
    worker()
  end
end
`)
	if got, want := p.Flow().Outcomes().Count(), 4*p.Source().Identity().FamilyCount(keyspace.FamilyBody); got != want {
		t.Fatalf("OutcomeCount = %d, want exactly four shared outcomes per Body (%d)", got, want)
	}
	entry, _ := p.Source().Index().Entry()
	outer, _ := p.Source().Order().BodyAt(entry, 0)
	inner, _ := p.Source().Order().BodyAt(outer, 0)
	call, ok := p.Flow().Authored().Calls().At(0)
	if !ok {
		t.Fatal("missing authored worker Call")
	}
	boundary, ok := p.Flow().Causal().Boundaries().For(call)
	next := boundary.Normal
	if !ok || next == 0 {
		t.Fatal("Call has no ordinary successor")
	}
	for _, outcome := range []struct {
		kind flowkind.OutcomeKind
		arm  func() keyspace.Term
	}{
		{flowkind.OutcomeThrow, func() keyspace.Term { return boundary.Throw }},
		{flowkind.OutcomeYield, func() keyspace.Term { return boundary.Yield }},
		{flowkind.OutcomeCancel, func() keyspace.Term { return boundary.Cancel }},
	} {
		immediate := outcome.arm()
		if immediate == 0 {
			t.Fatalf("Call has no %v exit", outcome.kind)
		}
		outerExit, ok := p.Flow().Outcomes().BodyExit(outer, outcome.kind)
		if !ok {
			t.Fatalf("outer Body has no %v exit", outcome.kind)
		}
		entryExit, ok := p.Flow().Outcomes().BodyExit(entry, outcome.kind)
		if !ok {
			t.Fatalf("entry Body has no %v exit", outcome.kind)
		}
		applicationOutcome(t, p, immediate, inner, outcome.kind, outerExit)
		applicationOutcome(t, p, outerExit, outer, outcome.kind, entryExit)
		applicationOutcome(t, p, entryExit, entry, outcome.kind, 0)
		if immediate == next {
			t.Fatalf("Call %v exit reused ordinary successor %v", outcome.kind, next)
		}
	}
}

func TestSourceApplicationOutcomesDoNotScaleWithCalls(t *testing.T) {
	const calls = 512
	var source strings.Builder
	source.Grow(calls * len("worker()\n"))
	for index := 0; index < calls; index++ {
		source.WriteString("worker()\n")
	}
	p := parseBindLower(t, source.String())
	if p.Flow().Authored().Calls().Count() != calls {
		t.Fatalf("CallCount = %d, want %d", p.Flow().Authored().Calls().Count(), calls)
	}
	if got, want := p.Flow().Outcomes().Count(), 4*p.Source().Identity().FamilyCount(keyspace.FamilyBody); got != want {
		t.Fatalf("OutcomeCount = %d, want shared O(Bodies) storage %d", got, want)
	}
}

func TestSourceApplicationGenericForOwnsImplicitIterator(t *testing.T) {
	p := parseBindLower(t, `
local function iterator(state, control)
  for value in iterator, state, control do
    local saved = value
  end
end
`)
	if p.Flow().Authored().Calls().Count() != 0 {
		t.Fatalf("CallCount = %d, want no synthetic iterator Call", p.Flow().Authored().Calls().Count())
	}
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, ok := p.Flow().Authored().Functions().Get(function)
	if !ok {
		t.Fatal("missing iterator Function")
	}
	loop, ok := p.Source().Order().BodyAt(body, 0)
	if !ok {
		t.Fatal("missing generic Loop")
	}
	owner, _, loopKind, header, ok := p.Flow().Authored().Control().Loops().Get(loop)
	if !ok || loopKind != flowkind.LoopGenericFor {
		t.Fatalf("Loop = kind %v ok %v, want generic", loopKind, ok)
	}
	if width, ok := p.Flow().Authored().Control().Loops().CellCount(loop); !ok || width != 1 {
		t.Fatalf("GenericLoopIterationWidth = %d/%v, want one loop Cell", width, ok)
	}
	if fixed, ok := p.Flow().Authored().Values().Len(header); !ok || fixed != 3 {
		t.Fatalf("generic header fixed source prefix = %d/%v, want generator/state/control", fixed, ok)
	}
	direct, ok := p.Flow().DirectFunctions().GenericLoop(loop)
	if !ok || direct != function {
		t.Fatalf("GenericLoopDirectFunction = %v/%v, want %v", direct, ok, function)
	}
	for _, outcomeKind := range []flowkind.OutcomeKind{flowkind.OutcomeThrow, flowkind.OutcomeYield, flowkind.OutcomeCancel} {
		if term, ok := p.Flow().Outcomes().BodyExit(owner, outcomeKind); !ok || term == 0 {
			t.Fatal("generic Loop lacks shared application exit")
		}
	}
}

func TestSourceApplicationGenericForOpenHeaderHasNoPhantomCall(t *testing.T) {
	p := parseBindLower(t, `for value in factory() do end`)
	if p.Flow().Authored().Calls().Count() != 1 {
		t.Fatalf("CallCount = %d, want only authored factory Call", p.Flow().Authored().Calls().Count())
	}
	entry, _ := p.Source().Index().Entry()
	loop, ok := p.Source().Order().BodyAt(entry, 0)
	if !ok {
		t.Fatal("missing generic Loop")
	}
	_, _, loopKind, header, ok := p.Flow().Authored().Control().Loops().Get(loop)
	if !ok || loopKind != flowkind.LoopGenericFor {
		t.Fatal("generic Loop lacks header Values")
	}
	if fixed, ok := p.Flow().Authored().Values().Len(header); !ok || fixed != 0 {
		t.Fatalf("open generic header fixed values = %d/%v, want none", fixed, ok)
	}
	_, tail, ok := p.Flow().Authored().Values().Get(header)
	if !ok || tail == 0 {
		t.Fatal("generic header lost authored open tail")
	}
	call, _ := p.Flow().Authored().Calls().At(0)
	if tail != call {
		t.Fatalf("generic header tail = %v, want authored factory Call %v", tail, call)
	}
	if direct, ok := p.Flow().DirectFunctions().GenericLoop(loop); ok || direct != 0 {
		t.Fatalf("open generic header direct iterator = %v/%v, want none", direct, ok)
	}
}

func TestSourceApplicationTerminalGenericIteratorDoesNotCreateDirectFunction(t *testing.T) {
	p := parseBindLower(t, `
local function iterator(state, control)
  do return end
  for value in iterator, state, control do end
end
`)
	function, _ := p.Flow().Authored().Functions().At(0)
	_, body, _, _ := p.Flow().Authored().Functions().Get(function)
	loop, ok := p.Source().Order().BodyAt(body, 1)
	if !ok {
		t.Fatal("missing unreachable generic Loop")
	}
	if direct, ok := p.Flow().DirectFunctions().GenericLoop(loop); ok || direct != 0 {
		t.Fatalf("unreachable generic Loop direct candidate = %v/%v, want none", direct, ok)
	}
}

func TestSourceApplicationCausalEntrySealIsDeepIterative(t *testing.T) {
	const depth = 4096
	var source strings.Builder
	source.Grow(depth*4 + len("local value = true"))
	source.WriteString("local value = ")
	for index := 0; index < depth; index++ {
		source.WriteString("not ")
	}
	source.WriteString("true")
	p := parseBindLower(t, source.String())
	if p == nil {
		t.Fatal("deep source did not seal a Program")
	}
}

// This source law is intentionally separate from the atomic cases:
// it proves the coupled closure facts that require one method, one capture,
// and one vararg in the same authored function.
func TestSourceApplicationMethodClosureLayout(t *testing.T) {
	p := parseBindLower(t, `
local captured = 1
local receiver = {}
function receiver:method(first, ...)
  return captured, self, first, ...
end
`)
	flow := p.Flow()
	function, ok := flow.Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing authored method Function")
	}
	_, _, vararg, ok := flow.Authored().Functions().Get(function)
	if !ok || vararg == 0 {
		t.Fatalf("method Function vararg = %v/%v", vararg, ok)
	}
	if formals, ok := p.Source().Formals().Len(function); !ok || formals != 2 {
		t.Fatalf("method Function formal count = %d/%v, want implicit self and first", formals, ok)
	}
	for index := 0; index < 2; index++ {
		formal, ok := p.Source().Formals().At(function, index)
		if !ok || formal == 0 {
			t.Fatalf("method formal %d = %v/%v", index, formal, ok)
		}
	}
	if _, cell, ok := flow.Authored().Storage().Varargs().Get(vararg); !ok || cell == 0 {
		t.Fatalf("method vararg Cell = %v/%v", cell, ok)
	}
	if captures, ok := flow.Authored().Functions().CaptureCount(function); !ok || captures != 1 {
		t.Fatalf("method capture count = %d/%v, want captured lexical value", captures, ok)
	}
	inner, outer, ok := flow.Authored().Functions().CaptureAt(function, 0)
	if !ok || inner == 0 || outer == 0 {
		t.Fatalf("method capture = %v/%v/%v", inner, outer, ok)
	}
}

// and/or have two distinct control continuations. The atomic cases prove
// their exact source relation; this paired source law proves both selected
// arms remain explicit in the sealed Program.
func TestSourceApplicationSelectKeepsBothArms(t *testing.T) {
	p := parseBindLower(t, `local left, right = true, false; return left and right, left or right`)
	flow := p.Flow()
	selects := flow.Authored().Operators().Selects()
	for index, want := range []kind.SelectOp{kind.SelectAnd, kind.SelectOr} {
		term, ok := selects.At(index)
		if !ok {
			t.Fatalf("missing Select %d", index)
		}
		_, op, _, _, ok := selects.Get(term)
		if !ok || op != want {
			t.Fatalf("Select %d operator = %v/%v, want %v", index, op, ok, want)
		}
		var truthy, falsy keyspace.Term
		successors := flow.Causal().Successors()
		for successorIndex := 0; successorIndex < successors.Count(term); successorIndex++ {
			successor, successorOK := successors.At(term, successorIndex)
			if !successorOK || successor.Decision != term {
				continue
			}
			if successor.Truth {
				truthy = successor.To
			} else {
				falsy = successor.To
			}
		}
		if truthy == 0 {
			t.Fatalf("Select %d has no truthy continuation", index)
		}
		if falsy == 0 {
			t.Fatalf("Select %d has no falsy continuation", index)
		}
	}
}

func sourceKeyText(t testing.TB, p *program.Program, key keyspace.Key) string {
	t.Helper()
	value, ok := p.Source().Keys().Exact(key)
	if !ok || value.Kind != keyspace.LiteralString {
		t.Fatalf("Source exact key = %#v/%v", value, ok)
	}
	return value.String
}

func TestFlowDirectBindingsKeepDirectSelectorCalls(t *testing.T) {
	p := parseBindLower(t, "\nlocal key = \"abs\"\nmath.abs(1)\nmath[key](2)\nmath:abs(3)\n")
	calls := p.Flow().Authored().Calls()
	bindings := p.Flow().DirectBindings()
	var plain, method keyspace.Term
	for index := 0; index < calls.Count(); index++ {
		call, _ := calls.At(index)
		read, form, ok := bindings.Call(call)
		if !ok {
			continue
		}
		root, depth, selected := bindings.Selection(read)
		if !selected || root == 0 || depth != 1 {
			t.Fatalf("direct Call[%d] selection = root %v depth %d ok %v", index, root, depth, selected)
		}
		_, _, key, cellOK := p.Flow().Authored().Storage().Cells().Get(root)
		if !cellOK || sourceKeyText(t, p, key) != "math" {
			t.Fatalf("direct Call[%d] root cell = %v/%v", index, root, cellOK)
		}
		switch form {
		case flow.CallFormPlain:
			plain = call
		case flow.CallFormMethod:
			method = call
		default:
			t.Fatalf("direct Call[%d] form = %v", index, form)
		}
	}
	if plain == 0 || method == 0 {
		t.Fatalf("plain/method direct calls = %v/%v, want both", plain, method)
	}
}

func TestModuleImportAndStaticPublicationUseTheirFinalOwners(t *testing.T) {
	p := parseBindLower(t, "\nlocal M = require(\"pkg.core\")\ntype User = M.Schema.User\nM.Schema.User = User\nreturn M\n")
	module := p.Module()
	if module.Count() != 1 {
		t.Fatalf("Module Import count = %d, want one", module.Count())
	}
	imported, ok := module.ImportAt(0)
	if !ok || imported.Call == 0 || imported.Alias == 0 || imported.Request == 0 || imported.Key == 0 {
		t.Fatalf("Module Import = %#v/%v", imported, ok)
	}
	request, _, text, requestOK := p.Source().Literals().Strings().At(int(keyspace.TermOrdinal(imported.Request) - 1))
	if !requestOK || request != imported.Request || text != "pkg.core" {
		t.Fatalf("Module Import request = %v/%q/%v", request, text, requestOK)
	}
	value, keyOK := p.Source().Keys().Exact(imported.Key)
	if !keyOK || value.Kind != keyspace.LiteralString || value.String != "pkg.core" {
		t.Fatalf("Module Import key = %#v/%v", value, keyOK)
	}
	publication, publicationOK := p.Static().Publications().At(0)
	if !publicationOK {
		t.Fatal("missing Static publication")
	}
	assign, pair, target, publicationRowOK := p.Static().Publications().Get(publication)
	if !publicationRowOK || assign == 0 || pair != 0 || target == 0 {
		t.Fatalf("Static publication = assign %v pair %d target %v ok %v", assign, pair, target, publicationRowOK)
	}
	root, owner, depth, bindingOK := p.Flow().DirectBindings().Publication(publication)
	if !bindingOK || root != imported.Alias || owner == 0 || depth != 2 {
		t.Fatalf("Flow publication binding = root %v owner %v depth %d ok %v", root, owner, depth, bindingOK)
	}
	path, pathOK := p.Flow().DirectBindings().PublicationPath(publication)
	if !pathOK {
		t.Fatal("missing Flow publication path cursor")
	}
	first, next, firstOK := path.Segment()
	second, _, secondOK := next.Segment()
	if !firstOK || !secondOK || sourceKeyText(t, p, first) != "T" || sourceKeyText(t, p, second) != "Schema" {
		t.Fatalf("Flow publication path = %v/%v %v/%v", first, firstOK, second, secondOK)
	}
}

func TestModuleEntryKeepsReturnedRootAndMembers(t *testing.T) {
	p := parseBindLower(t, "return { api = { f = function() end }, value = 1 }")
	entry := p.Module().Entry()
	returned, ok := entry.ReturnAt(0)
	if !ok || returned == 0 {
		t.Fatalf("Module Entry return = %v/%v", returned, ok)
	}
	if count, ok := entry.MemberCount(returned); !ok || count != 2 {
		t.Fatalf("Module Entry member count = %d/%v, want 2", count, ok)
	}
	for index := 0; index < 2; index++ {
		member, memberOK := entry.MemberAt(returned, index)
		if !memberOK || member == 0 {
			t.Fatalf("Module Entry member[%d] = %v/%v", index, member, memberOK)
		}
	}
}

func TestFlowDirectBindingDeepSelectorPathIsAllocationFree(t *testing.T) {
	const depth = 256
	var input strings.Builder
	input.WriteString("api")
	for index := 0; index < depth; index++ {
		input.WriteString(".x")
		input.WriteString(strconv.Itoa(index))
	}
	input.WriteString("()")
	p := parseBindLower(t, input.String())
	call, ok := p.Flow().Authored().Calls().At(0)
	if !ok {
		t.Fatal("missing Call")
	}
	read, form, bindingOK := p.Flow().DirectBindings().Call(call)
	if !bindingOK || form != flow.CallFormPlain {
		t.Fatalf("direct Call binding = read %v form %v ok %v", read, form, bindingOK)
	}
	if _, gotDepth, ok := p.Flow().DirectBindings().Selection(read); !ok || gotDepth != depth {
		t.Fatalf("deep selector depth = %d/%v, want %d", gotDepth, ok, depth)
	}
	if allocations := testing.AllocsPerRun(1000, func() {
		path, _ := p.Flow().DirectBindings().SelectionPath(read)
		for {
			_, next, ok := path.Segment()
			if !ok {
				break
			}
			path = next
		}
	}); allocations != 0 {
		t.Fatalf("deep selector cursor allocations = %v, want 0", allocations)
	}
}

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

func controlSourceAt(t *testing.T, p *program.Program, body keyspace.Term, index int) keyspace.Term {
	t.Helper()
	term, ok := p.Source().Order().BodyAt(body, index)
	if !ok {
		t.Fatalf("Source Order BodyAt(%v, %d) is absent", body, index)
	}
	return term
}

func TestSourceControlKeepsBranchBodiesInSourceAndFlowOwners(t *testing.T) {
	p := parseBindLower(t, "\nif first() then\n  return 1\nelseif second() then\n  return 2\nelse\n  return 3\nend\n")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry")
	}
	outer := controlSourceAt(t, p, entry, 0)
	branches := p.Flow().Authored().Control().Branches()
	owner, condition, whenTrue, whenFalse, branchOK := branches.Get(outer)
	if !branchOK || owner != entry || condition == 0 || whenTrue == 0 || whenFalse == 0 {
		t.Fatalf("Branch = owner %v condition %v true %v false %v ok %v", owner, condition, whenTrue, whenFalse, branchOK)
	}
	if parent, ok := p.Source().Index().BodyParent(whenTrue); !ok || parent != entry {
		t.Fatalf("truthy Body parent = %v/%v, want %v", parent, ok, entry)
	}
	inner := controlSourceAt(t, p, whenFalse, 0)
	innerOwner, innerCondition, innerTrue, innerFalse, innerOK := branches.Get(inner)
	if !innerOK || innerOwner != whenFalse || innerCondition == 0 || innerTrue == 0 || innerFalse == 0 {
		t.Fatalf("elseif Branch = owner %v condition %v true %v false %v ok %v", innerOwner, innerCondition, innerTrue, innerFalse, innerOK)
	}
	if parent, ok := p.Source().Index().BodyParent(innerTrue); !ok || parent != whenFalse {
		t.Fatalf("elseif truthy Body parent = %v/%v, want %v", parent, ok, whenFalse)
	}
}

func TestFlowControlRowsCoverEveryLuaLoopForm(t *testing.T) {
	for _, sample := range []struct {
		name  string
		input string
		kind  kind.LoopKind
		cells int
	}{
		{"while", "while test() do local value = 1 end", kind.LoopWhile, 0},
		{"repeat", "repeat local value = 1 until test()", kind.LoopRepeat, 0},
		{"numeric", "for i = 1, 2, 1 do local value = i end", kind.LoopNumericFor, 1},
		{"generic", "for key, value in iterate() do local seen = key end", kind.LoopGenericFor, 2},
	} {
		t.Run(sample.name, func(t *testing.T) {
			p := parseBindLower(t, sample.input)
			loops := p.Flow().Authored().Control().Loops()
			loop, ok := loops.At(0)
			if !ok {
				t.Fatal("missing Loop")
			}
			owner, body, loopKind, control, rowOK := loops.Get(loop)
			if !rowOK || owner == 0 || body == 0 || control == 0 || loopKind != sample.kind {
				t.Fatalf("Loop = owner %v body %v kind %v control %v ok %v", owner, body, loopKind, control, rowOK)
			}
			if sample.cells != 0 {
				if count, ok := loops.CellCount(loop); !ok || count != sample.cells {
					t.Fatalf("Loop CellCount = %d/%v, want %d", count, ok, sample.cells)
				}
			}
			normal, normalOK := p.Flow().Outcomes().BodyExit(body, kind.OutcomeNormal)
			if !normalOK || normal == 0 {
				t.Fatal("Loop Body has no normal Outcome")
			}
			found := false
			edges := p.Flow().Causal().Edges()
			for index := 0; index < edges.Count(); index++ {
				edge, edgeOK := edges.At(index)
				found = found || edgeOK && edge.Mu == loop
			}
			if !found {
				t.Fatal("Loop has no final recurrence Edge")
			}
		})
	}
}

func TestFlowControlOutcomesKeepBreakAndGotoTyped(t *testing.T) {
	p := parseBindLower(t, "\n::again::\nwhile test() do break end\ngoto again\n")
	control := p.Flow().Authored().Control()
	loop, loopOK := control.Loops().At(0)
	breakTerm, breakOK := control.Breaks().At(0)
	jump, gotoOK := control.Gotos().At(0)
	label, labelOK := control.Labels().At(0)
	if !loopOK || !breakOK || !gotoOK || !labelOK {
		t.Fatalf("control rows loop=%v/%v break=%v/%v goto=%v/%v label=%v/%v", loop, loopOK, breakTerm, breakOK, jump, gotoOK, label, labelOK)
	}
	if target, ok := control.Breaks().Get(breakTerm); !ok || target != loop {
		t.Fatalf("Break target = %v/%v, want Loop %v", target, ok, loop)
	}
	if _, target, ok := control.Gotos().Get(jump); !ok || target != label {
		t.Fatalf("Goto target = %v/%v, want Label %v", target, ok, label)
	}
	breakExit, breakExitOK := p.Flow().Outcomes().BreakExit(breakTerm)
	jumpExit, jumpExitOK := p.Flow().Outcomes().GotoExit(jump)
	if !breakExitOK || !jumpExitOK || breakExit == 0 || jumpExit == 0 {
		t.Fatalf("control exits break=%v/%v goto=%v/%v", breakExit, breakExitOK, jumpExit, jumpExitOK)
	}
}

func TestFlowDirectFunctionsFilterDeadCallsWithoutRootDecisionPlane(t *testing.T) {
	dead := parseBindLower(t, "local function f() goto done; f(); ::done:: end")
	function, functionOK := dead.Flow().Authored().Functions().At(0)
	call, callOK := dead.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatalf("dead Function/Call = %v/%v %v/%v", function, functionOK, call, callOK)
	}
	if direct, ok := dead.Flow().DirectFunctions().Call(call); ok || direct != 0 {
		t.Fatalf("dead Call direct Function = %v/%v, want absent", direct, ok)
	}
	if dead.Flow().Executable().Contains(call) {
		t.Fatal("dead Call became executable")
	}

	live := parseBindLower(t, "local function f() return f() end")
	function, functionOK = live.Flow().Authored().Functions().At(0)
	call, callOK = live.Flow().Authored().Calls().At(0)
	if !functionOK || !callOK {
		t.Fatalf("live Function/Call = %v/%v %v/%v", function, functionOK, call, callOK)
	}
	if direct, ok := live.Flow().DirectFunctions().Call(call); !ok || direct != function {
		t.Fatalf("live Call direct Function = %v/%v, want %v", direct, ok, function)
	}
}

func TestSourceControlFaultsStaySourceOwnedStaticEvidence(t *testing.T) {
	p := parseBindLower(t, "type Snapshot = typeof(function() goto missing end)")
	entry, ok := p.Source().Index().Entry()
	if !ok {
		t.Fatal("missing static function Body")
	}
	fault := controlSourceAt(t, p, entry, 0)
	got, faultOK := p.Source().Faults().At(fault)
	if !faultOK || got.Owner != entry || got.Kind != source.ControlFaultUndefinedGoto {
		t.Fatalf("Source ControlFault = %#v/%v", got, faultOK)
	}
	if !p.Flow().Containment().Static(fault) {
		t.Fatal("static ControlFault escaped static containment")
	}
	if got := p.Flow().Authored().Control().Gotos().Count(); got != 0 {
		t.Fatalf("static invalid Goto became executable: %d", got)
	}
}

func TestSourceDirectFunctionProofRequiresInstallationOnEveryPath(t *testing.T) {
	directAt := func(t *testing.T, p *program.Program, index int) keyspace.Term {
		t.Helper()
		flow := p.Flow()
		call, ok := flow.Authored().Calls().At(index)
		if !ok {
			t.Fatalf("missing Call %d", index)
		}
		function, _ := flow.DirectFunctions().Call(call)
		return function
	}

	t.Run("assignment after the Call cannot prove it", func(t *testing.T) {
		p := parseBindLower(t, `
local f
f()
f = function() end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != 0 {
			t.Fatalf("pre-install Call direct = %v, want none", got)
		}
		if got := directAt(t, p, 1); got != function {
			t.Fatalf("post-install Call direct = %v, want %v", got, function)
		}
	})

	t.Run("unconditional do installation is direct after its Body", func(t *testing.T) {
		p := parseBindLower(t, `
local f
do f = function() end end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("do-installed Call direct = %v, want %v", got, function)
		}
	})

	t.Run("captured Cell installation is direct in the same activation", func(t *testing.T) {
		p := parseBindLower(t, `
local f
local g = function()
	f = function() end
	f()
end
g()
`)
		installed, _ := p.Flow().Authored().Functions().At(1)
		if got := directAt(t, p, 0); got != installed {
			t.Fatalf("same-activation captured Call direct = %v, want %v", got, installed)
		}
	})

	for _, test := range []struct {
		name, source string
	}{
		{
			name: "multi-assign RHS runs before its writes",
			source: `local f, x
f, x = function() end, f()`,
		},
		{
			name: "extra assignment RHS Call runs before its write",
			source: `local f
f = function() end, f()`,
		},
		{
			name: "Call argument in assignment RHS runs before its write",
			source: `local f
f = function() end, invoke(f())`,
		},
		{
			name: "table field in assignment RHS runs before its write",
			source: `local f, value
f, value = function() end, { callback = f() }`,
		},
		{
			name:   "local initializer cannot see its own installation",
			source: `local f, value = function() end, f()`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			for index := 0; index < p.Flow().Authored().Calls().Count(); index++ {
				if got := directAt(t, p, index); got != 0 {
					t.Fatalf("same-RHS Call %d direct = %v, want none", index, got)
				}
			}
		})
	}

	t.Run("cross-activation installation is not direct", func(t *testing.T) {
		p := parseBindLower(t, `
local f
local install = function() f = function() end end
install()
f()
`)
		if got := directAt(t, p, 1); got != 0 {
			t.Fatalf("cross-activation Call direct = %v, want none", got)
		}
	})

	for _, test := range []struct {
		name, source string
	}{
		{
			name: "branch may bypass installation",
			source: `local f
if condition then f = function() end end
f()`,
		},
		{
			name: "loop may execute zero times",
			source: `local f
while condition do f = function() end end
f()`,
		},
		{
			name: "goto bypasses installation",
			source: `local f
goto after
f = function() end
::after::
f()`,
		},
		{
			name: "capture has another activation root",
			source: `local f
local g = function() f() end
f = function() end
g()`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			p := parseBindLower(t, test.source)
			if got := directAt(t, p, 0); got != 0 {
				t.Fatalf("unproven Call direct = %v, want none", got)
			}
		})
	}

	t.Run("recursive local declaration remains exact", func(t *testing.T) {
		p := parseBindLower(t, `local function f() f() end`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("recursive Call direct = %v, want %v", got, function)
		}
	})

	t.Run("sole assignment closure retains recursive exactness", func(t *testing.T) {
		p := parseBindLower(t, `
local f
f = function() f() end
f()
`)
		function, _ := p.Flow().Authored().Functions().At(0)
		if got := directAt(t, p, 0); got != function {
			t.Fatalf("assignment-recursive Call direct = %v, want %v", got, function)
		}
		if got := directAt(t, p, 1); got != function {
			t.Fatalf("post-assignment Call direct = %v, want %v", got, function)
		}
	})
}
