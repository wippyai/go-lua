package lint

import (
	"context"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/shapefact"
)

func checkSingleModule(t *testing.T, source string) ProjectResult {
	t.Helper()
	result, err := CheckProject(context.Background(), ProjectInput{Entries: []Entry{{
		Path:       "main.lua",
		ModulePath: "main",
		Source:     source,
	}}})
	if err != nil {
		t.Fatalf("CheckProject: %v", err)
	}
	return result
}

func diagnosticSummaries(result ProjectResult) []string {
	out := make([]string, 0, len(result.Diagnostics))
	for _, item := range result.Diagnostics {
		out = append(out, RenderDiagnostic(item))
	}
	return out
}

func requireDiagnostic(t *testing.T, result ProjectResult, line int, contains string) {
	t.Helper()
	for _, item := range result.Diagnostics {
		if item.Position.Line == line && strings.Contains(item.Message, contains) {
			return
		}
	}
	t.Fatalf("no diagnostic at line %d containing %q; got %#v", line, contains, diagnosticSummaries(result))
}

func requireNoDiagnosticAt(t *testing.T, result ProjectResult, line int) {
	t.Helper()
	for _, item := range result.Diagnostics {
		if item.Position.Line == line {
			t.Fatalf("unexpected diagnostic at line %d: %s", line, RenderDiagnostic(item))
		}
	}
}

// A truthiness guard over a published optional witness refines the guarded path
// on the true edge, and every claim the arm makes is still checked. The unsound
// claim is the adversarial half: an arm the engine drops because it cannot
// decide the guard accepts it silently.
func TestTruthyGuardRefinesTheTrueEdgeAndStillChecksItsArm(t *testing.T) {
	result := checkSingleModule(t, `local function parse(text: string): number?
    return tonumber(text)
end

local value = parse("FF")
if value then
    local proven: number = value
    local unsound: string = value
    return proven, unsound
end
return nil, nil
`)
	requireNoDiagnosticAt(t, result, 7)
	requireDiagnostic(t, result, 8, "not string")
}

// The falsy edge of the same guard is a proof in its own right: past an arm that
// returns, the guarded path is exactly the falsy projection of its witness.
func TestTruthyGuardKeepsItsFalsyEdge(t *testing.T) {
	result := checkSingleModule(t, `local function parse(text: string): number?
    return tonumber(text)
end

local value = parse("FF")
if value then
    return value
end
local unsound: number = value
return unsound
`)
	requireDiagnostic(t, result, 9, "nil")
}

// A member read under a truthiness guard is still an obligation the arm owns.
func TestTruthyGuardChecksAGuardedMemberArm(t *testing.T) {
	result := checkSingleModule(t, `local function parse(text: string): number?
    return tonumber(text)
end

local holder = {slot = parse("1")}
if holder.slot then
    local unsound: string = holder.slot
    return unsound
end
return nil
`)
	requireDiagnostic(t, result, 7, "not string")
}

// A loop in a returned closure changes nothing the placement projection reads:
// the captured module table is actor-local state either way, and a declared
// scalar local is frame storage either way.
func TestPlacementProjectsAReturnedClosureThatLoops(t *testing.T) {
	result := checkSingleModule(t, `local cache: {[string]: string} = {}

local function main(batch: {string}): number
    local processed = 0
    for _, item in ipairs(batch) do
        local key: string = item
        cache[key] = item
        processed = processed + 1
    end
    return processed
end

return main
`)
	if result.Placement == nil {
		t.Fatal("placement = nil, want the returned closure's projected witnesses")
	}
	ownedTable, scalarDeclaration := false, false
	for _, allocation := range result.Placement.Allocations {
		if allocation.Kind == "lua.table" && allocation.Placement.String() == "owned-heap" && allocation.OwnerIdentity {
			ownedTable = true
		}
		if allocation.Kind == "lua.scalar" && allocation.Placement.String() == "stack" && allocation.FrameLocal &&
			shapefact.IsScalarKind([]byte(allocation.Identity), shapefact.ScalarDeclaration) {
			scalarDeclaration = true
		}
	}
	if !ownedTable {
		t.Fatalf("placement = %#v, want the captured module table owned by the returned closure", result.Placement.Allocations)
	}
	if !scalarDeclaration {
		t.Fatalf("placement = %#v, want a frame-local witness for the declared scalar local", result.Placement.Allocations)
	}
}
