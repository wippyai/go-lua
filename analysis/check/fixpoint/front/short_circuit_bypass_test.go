package front_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/front"
)

// branchBypassOperands collects, for every branch a body lowers, the operands
// naming the value-position short-circuit that branch guards. A branch that
// guards none contributes nothing.
func branchBypassOperands(t *testing.T, source string) []map[string]string {
	t.Helper()
	compilation, err := front.Compile(source)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	var out []map[string]string
	for _, operation := range compilation.Artifact.Equations {
		if operation.Occurrence.Kind != "branch-relations" {
			continue
		}
		stated := map[string]string{}
		for _, operand := range operation.Operands {
			switch operand.Role {
			case "short-circuit-result", "short-circuit-operand", "short-circuit-bypass":
				stated[operand.Role] = string(operand.Term.Encoding)
			}
		}
		if len(stated) != 0 {
			out = append(out, stated)
		}
	}
	return out
}

// TestAndGuardNamesItsFalseEdgeAsTheBypass states the topology the engine reads:
// the result cell is written before the guard because the bypass edge is the
// join and owns no point, and `and` reaches that edge when its left operand is
// falsy.
func TestAndGuardNamesItsFalseEdgeAsTheBypass(t *testing.T) {
	stated := branchBypassOperands(t, `type Entry = {meta: {suite: string?}?}
local e: Entry = {}
local suite = e.meta and e.meta.suite
`)
	if len(stated) != 1 {
		t.Fatalf("short-circuit guards = %d, want 1", len(stated))
	}
	if got := stated[0]["short-circuit-bypass"]; got != "scalar/bool/false" {
		t.Fatalf("and bypass edge = %q, want the false edge", got)
	}
	if got := stated[0]["short-circuit-operand"]; got != "path/sym2.meta" {
		t.Fatalf("and bypass operand = %q, want the left operand path", got)
	}
	if got := stated[0]["short-circuit-result"]; got != "temp/0" {
		t.Fatalf("and bypass result = %q, want the short-circuit result cell", got)
	}
}

// TestOrGuardNamesItsTrueEdgeAsTheBypass is the mirror: `or` yields its left
// operand exactly when that operand is truthy.
func TestOrGuardNamesItsTrueEdgeAsTheBypass(t *testing.T) {
	stated := branchBypassOperands(t, `type User = {name: string, nick: string?}
local u: User = {name = "n"}
local shown = u.nick or u.name
`)
	if len(stated) != 1 {
		t.Fatalf("short-circuit guards = %d, want 1", len(stated))
	}
	if got := stated[0]["short-circuit-bypass"]; got != "scalar/bool/true" {
		t.Fatalf("or bypass edge = %q, want the true edge", got)
	}
	if got := stated[0]["short-circuit-operand"]; got != "path/sym2.nick" {
		t.Fatalf("or bypass operand = %q, want the left operand path", got)
	}
}

// TestPointLocalShortCircuitStatesNoBypass keeps the two lowerings apart: a
// conservatively pure right operand keeps both operands in one expression, whose
// own kernel owns the result, so its guard names no result cell.
func TestPointLocalShortCircuitStatesNoBypass(t *testing.T) {
	stated := branchBypassOperands(t, `type User = {name: string, nick: string?}
local u: User = {name = "n"}
local shown = u.nick or "fallback"
`)
	if len(stated) != 0 {
		t.Fatalf("short-circuit guards for a point-local form = %v, want none", stated)
	}
}
