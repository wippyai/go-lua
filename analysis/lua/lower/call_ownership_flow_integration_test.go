package lower_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	flowkind "github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
	flowView := p.Flow()
	function, ok := flowView.Authored().Functions().At(0)
	if !ok {
		t.Fatal("missing authored method Function")
	}
	_, _, vararg, ok := flowView.Authored().Functions().Get(function)
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
	cellKind, body, key, cellOK := flowView.Authored().Storage().Cells().Get(vararg)
	if !cellOK || cellKind != flow.CellLocal || body == 0 || key != 0 {
		t.Fatalf("method vararg Cell = kind %v body %v key %v ok %v", cellKind, body, key, cellOK)
	}
	varargOccurrence, occurrenceOK := flowView.Authored().Storage().Varargs().At(0)
	if !occurrenceOK {
		t.Fatal("missing method vararg occurrence")
	}
	if occurrenceOwner, cell, ok := flowView.Authored().Storage().Varargs().Get(varargOccurrence); !ok || occurrenceOwner != body || cell != vararg {
		t.Fatalf("method vararg occurrence = owner %v Cell %v ok %v, want owner %v Cell %v", occurrenceOwner, cell, ok, body, vararg)
	}
	if captures, ok := flowView.Authored().Functions().CaptureCount(function); !ok || captures != 1 {
		t.Fatalf("method capture count = %d/%v, want captured lexical value", captures, ok)
	}
	inner, outer, ok := flowView.Authored().Functions().CaptureAt(function, 0)
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
		_, op, left, _, ok := selects.Get(term)
		if !ok || op != want {
			t.Fatalf("Select %d operator = %v/%v, want %v", index, op, ok, want)
		}
		var truthy, falsy keyspace.Term
		successors := flow.Causal().Successors()
		for successorIndex := 0; successorIndex < successors.Count(left); successorIndex++ {
			successor, successorOK := successors.At(left, successorIndex)
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
		if truthy == falsy {
			t.Fatalf("Select %d collapsed its truthy and falsy continuations to %v", index, truthy)
		}
	}
}
