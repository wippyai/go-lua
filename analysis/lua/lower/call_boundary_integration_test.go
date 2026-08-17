package lower_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
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
