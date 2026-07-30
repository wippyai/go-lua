package engine_test

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/engine"
)

// TestNilPredicateOnOptionalWitnessKeepsBothArms proves that a nil test over a
// value known only as a type witness selects neither arm on its own. The
// witness for `string?` denotes both nil and a string, so the guarded arm stays
// reachable and every obligation it carries is still owed.
func TestNilPredicateOnOptionalWitnessKeepsBothArms(t *testing.T) {
	result, err := engine.Check(`
local function f(x: string?)
    local present: string = x
    if x == nil then
        local wrong: number = "boom"
    end
end
f(nil)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Span.StartLine == 5 && strings.Contains(diagnostic.Message, "wrong") {
			return
		}
	}
	t.Fatalf("nil test over an optional witness left its arm unchecked: %#v", result.PublishedDiagnostics)
}

// TestNotNilPredicateOnOptionalWitnessNarrowsItsTrueArm proves the same test
// keeps its refinement: the true arm of `x ~= nil` reads x without nil, while
// the value after the join keeps it.
func TestNotNilPredicateOnOptionalWitnessNarrowsItsTrueArm(t *testing.T) {
	result, err := engine.Check(`
local function f(x: string?)
    if x ~= nil then
        local narrowed: string = x
    end
    local joined: string = x
end
f(nil)`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	joined := false
	for _, diagnostic := range result.PublishedDiagnostics {
		switch diagnostic.Span.StartLine {
		case 4:
			t.Fatalf("guarded read of a narrowed optional was refuted: %#v", diagnostic)
		case 6:
			joined = true
		}
	}
	if !joined {
		t.Fatalf("unguarded read of an optional past the join was not refuted: %#v", result.PublishedDiagnostics)
	}
}

// TestLiteralPredicateOnUnionWitnessKeepsBothArms proves a literal comparison
// against a value whose witness admits that literal decides nothing by itself:
// a type denotes a set of values, so byte inequality between a witness and a
// literal encoding is not value inequality.
func TestLiteralPredicateOnUnionWitnessKeepsBothArms(t *testing.T) {
	result, err := engine.Check(`
local function f(tag: "ok" | "err")
    local seen: string = tag
    if tag == "ok" then
        local wrong: number = "boom"
    end
end
f("ok")`)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, diagnostic := range result.PublishedDiagnostics {
		if diagnostic.Span.StartLine == 5 && strings.Contains(diagnostic.Message, "wrong") {
			return
		}
	}
	t.Fatalf("literal test over a union witness left its arm unchecked: %#v", result.PublishedDiagnostics)
}
