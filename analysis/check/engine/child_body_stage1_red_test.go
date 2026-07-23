package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

func TestStage1RedUncalledChildDiagnostics(t *testing.T) {
	r := checkChildAdmission(t, `local f = function() local bad: string = 1 end`)
	if len(r.Diagnostics) != 1 || !strings.HasPrefix(r.Diagnostics[0].Key, "child/") || string(r.Diagnostics[0].Value) != "cannot assign bad because it is number, not string" || !r.DiagnosticSpans[r.Diagnostics[0].Key].Valid() {
		t.Fatalf("uncalled child diagnostic/spans = %#v / %#v", r.Diagnostics, r.DiagnosticSpans)
	}
}

func TestStage1RedThreeLevelCaptures(t *testing.T) {
	r := checkChildAdmission(t, `local x = 1; return function() return function() return function() return x end end end`)
	if got := valuesByName(r.Diagnostics); len(got) != 0 || valuesByName(r.Outcomes)["return/arity"] != "1" || !strings.HasPrefix(valuesByName(r.Outcomes)["return/0"], "scalar/function/") {
		t.Fatalf("three-level closure outcome = diagnostics %#v outcomes %#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedSiblingSharedMutation(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local inc = function() x = x + 1 end; local read = function() return x end; inc(); return read()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/0"] != "1" {
		t.Fatalf("sibling cell did not converge to one: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedReturnedClosures(t *testing.T) {
	r := checkChildAdmission(t, `local function make(x) return function() return x end end; local f = make(1); return f()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/0"] != "1" {
		t.Fatalf("returned closure lost its captured cell: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedMutableCaptureWriteback(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local set = function() x = 2 end; set(); local y: number = x`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Values)["y"] != "2" {
		t.Fatalf("capture writeback was not visible to caller: diagnostics=%#v values=%#v", r.Diagnostics, r.Values)
	}
}

func TestStage1RedArgumentCaptureAliasing(t *testing.T) {
	r := checkChildAdmission(t, `local x = {n = 0}; local f = function(a) x.n = 1; a.n = 2 end; f(x); local y: number = x.n`)
	if len(r.Diagnostics) != 0 {
		t.Fatalf("aliasing generated a speculative diagnostic: %#v", r.Diagnostics)
	}
}

func TestStage1RedSelfAndMutualRecursion(t *testing.T) {
	r := checkChildAdmission(t, `local f; local g; f = function() return g() end; g = function() return f() end; return f()`)
	if len(r.Diagnostics) != 0 || valuesByName(r.Outcomes)["return/arity"] != "1" {
		t.Fatalf("recursive lexical SCC did not close cleanly: diagnostics=%#v outcomes=%#v", r.Diagnostics, r.Outcomes)
	}
}

func TestStage1RedMixedKnownUnknownTargets(t *testing.T) {
	r := checkChildAdmission(t, `local f = function() return 1 end; local g = unknown and f or provider; return g()`)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Key != "analysis/conservative" || len(r.Values) != 0 || len(r.Outcomes) != 0 {
		t.Fatalf("mixed target must fail closed atomically: %#v", r)
	}
}

func TestStage1RedIncompleteEntryAtomicity(t *testing.T) {
	r := checkChildAdmission(t, `local x = 0; local f = function(a) x = a end; f(provider())`)
	if len(r.Diagnostics) != 1 || r.Diagnostics[0].Key != "analysis/conservative" || len(r.Values) != 0 || len(r.Outcomes) != 0 {
		t.Fatalf("failed projection leaked partial facts: %#v", r)
	}
}

func checkChildAdmission(t *testing.T, source string) engine.Result {
	t.Helper()
	result, err := engine.Check(source)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	return result
}
