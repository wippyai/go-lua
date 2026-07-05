package transferfacts

import (
	"reflect"
	"testing"

	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestWIRBranchChecksMatchSemanticDirectBranchChecks(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: any, y: string?, i: integer, xs: {string})
    if x then local a = 1 end
    if y == nil then local b = 1 end
    if type(x) == "string" then local c = 1 end
    if x == "ready" then local d = 1 end
    if i >= 1 then local e = 1 end
    if i <= #xs then local g = 1 end
end
`, "type")
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	if body == nil {
		t.Fatal("wirlower returned nil body")
	}

	var checked int
	for _, point := range built.Graph.RPO() {
		fact, ok := result.BranchCondition(point)
		if !ok || fact.Check.Kind == branchcond.CheckNone {
			continue
		}
		checked++
		assertWIRBranchCheckContains(t, body, point, fact.Check)
	}
	if checked != 6 {
		t.Fatalf("checked %d direct branch conditions, want 6", checked)
	}
}

func TestLowerWithWIRDirectBranchChecksMatchesSidecarLowering(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: any, y: string?, i: integer, xs: {string})
    if x then local a = 1 end
    if y == nil then local b = 1 end
    if type(x) == "string" then local c = 1 end
    if x == "ready" then local d = 1 end
    if i >= 1 then local e = 1 end
    if i <= #xs then local g = 1 end
end
`, "type")
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, point := range built.Graph.RPO() {
		assertEqualBranchFacts(t, point, "condition source", branchConditionSourceForCompare(sidecarFacts, point), branchConditionSourceForCompare(wirFacts, point))
		assertEqualBranchFacts(t, point, "refinements", sidecarFacts.BranchRefinements(point), wirFacts.BranchRefinements(point))
		assertEqualBranchFacts(t, point, "len floors", sidecarFacts.BranchLenRefinements(point), wirFacts.BranchLenRefinements(point))
		assertEqualBranchFacts(t, point, "num floors", sidecarFacts.BranchNumFloorRefinements(point), wirFacts.BranchNumFloorRefinements(point))
		assertEqualBranchFacts(t, point, "path evidence", sidecarFacts.BranchPathEvidence(point), wirFacts.BranchPathEvidence(point))
	}
}

func TestLowerWithWIRCorrelationBranchChecksMatchesSidecarLowering(t *testing.T) {
	t.Run("protected_call", func(t *testing.T) {
		stmts, bindings, built, result := parseSemanticChunk(t, `
local function run_tests(): number
    return 1
end

local ok, result = pcall(run_tests)
if not ok then
    return
end
`, "pcall")
		body := wirlower.Lower("chunk", stmts, bindings, built)
		assertWIRBranchFactParity(t, built, result, body, bindings)
	})

	t.Run("type_is", func(t *testing.T) {
		stmts, bindings, built, result := parseSemanticChunk(t, `
type Point = {x: number, y: number}

local data: any = {}
local validated, err = Point:is(data)
if err then
    local failed = true
end
`)
		body := wirlower.Lower("chunk", stmts, bindings, built)
		assertWIRBranchFactParity(t, built, result, body, bindings)
	})
}

func assertWIRBranchFactParity(t *testing.T, built *cfgbuild.Result, result *semantics.Result, body *wir.Body, bindings *bind.Result) {
	t.Helper()
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	for _, point := range built.Graph.RPO() {
		assertEqualBranchFacts(t, point, "refinements", sidecarFacts.BranchRefinements(point), wirFacts.BranchRefinements(point))
		assertEqualBranchFacts(t, point, "presence relations", sidecarFacts.BranchPresenceRelations(point), wirFacts.BranchPresenceRelations(point))
	}
}

func assertEqualBranchFacts(t *testing.T, point cfg.Point, label string, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s at point %d mismatch\n got: %#v\nwant: %#v", label, point, got, want)
	}
}

type branchConditionSourceCompare struct {
	source factflow.ValueSource
	ok     bool
}

func branchConditionSourceForCompare(facts factflow.Facts, point cfg.Point) branchConditionSourceCompare {
	source, ok := facts.BranchConditionSource(point)
	return branchConditionSourceCompare{source: source, ok: ok}
}

func assertWIRBranchCheckContains(t *testing.T, body *wir.Body, point cfg.Point, want branchcond.Check) {
	t.Helper()
	for _, got := range body.BranchChecks(point) {
		if wirCheckMatchesBranchcond(got, want) {
			return
		}
	}
	t.Fatalf("wir branch checks at point %d = %#v, want semantic check %#v", point, body.BranchChecks(point), want)
}

func wirCheckMatchesBranchcond(got wir.Check, want branchcond.Check) bool {
	if branchcond.CheckKind(got.Kind) != want.Kind {
		return false
	}
	if !got.Path.Equal(want.Path) || !got.OtherPath.Equal(want.OtherPath) {
		return false
	}
	if got.TypeName != want.TypeName || got.LiteralString != want.LiteralString {
		return false
	}
	if got.LenFloor != want.LenFloor || got.NumFloor != want.NumFloor || got.Negated != want.Negated {
		return false
	}
	if got.Literal == nil || want.Literal == nil {
		return got.Literal == nil && want.Literal == nil
	}
	return typ.TypeEquals(got.Literal, want.Literal)
}
