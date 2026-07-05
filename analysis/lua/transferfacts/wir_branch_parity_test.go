package transferfacts

import (
	"reflect"
	"testing"

	path "github.com/wippyai/go-lua/analysis/domain/path"
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
	"github.com/wippyai/go-lua/compiler/ast"
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
    if table.isfrozen(xs) then local h = 1 end
end
`, "type", "table")
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
	if checked != 7 {
		t.Fatalf("checked %d direct branch conditions, want 7", checked)
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
		if fact, ok := result.BranchCondition(point); ok && fact.Check.Kind == branchcond.CheckTruthy {
			assertWIRTruthyBranchConditionSource(t, point, wirFacts, fact.Check.Path)
		} else {
			assertEqualBranchFacts(t, point, "condition source", branchConditionSourceForCompare(sidecarFacts, point), branchConditionSourceForCompare(wirFacts, point))
		}
		assertEqualBranchFacts(t, point, "refinements", sidecarFacts.BranchRefinements(point), wirFacts.BranchRefinements(point))
		assertEqualBranchFacts(t, point, "len floors", sidecarFacts.BranchLenRefinements(point), wirFacts.BranchLenRefinements(point))
		assertEqualBranchFacts(t, point, "num floors", sidecarFacts.BranchNumFloorRefinements(point), wirFacts.BranchNumFloorRefinements(point))
		assertEqualBranchFacts(t, point, "path evidence", sidecarFacts.BranchPathEvidence(point), wirFacts.BranchPathEvidence(point))
	}
}

func TestDirectBranchCheckFromWIRDoesNotRequireSemanticSidecarMatch(t *testing.T) {
	point := cfg.Point(1)
	body := wir.NewBody("branch")
	x := path.NewPath(1, "x")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{Kind: wir.CheckTruthy, Path: x}),
	})
	body.SetPointRange(point, start, start+1)

	got, ok := (&lowerer{wir: body}).directBranchCheckFromWIR(point)
	if !ok {
		t.Fatal("missing WIR direct branch check")
	}
	if got.Kind != branchcond.CheckTruthy || !got.Path.Equal(x) {
		t.Fatalf("WIR direct branch check = %#v, want truthy x", got)
	}
}

func TestLowerWithWIRCompoundBranchPathRelationsMatchSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local a, b, c, d = {}, {}, {}, {}
if a == b and c ~= d then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	want := sidecarFacts.BranchPathRelations(point)
	if len(want) != 2 {
		t.Fatalf("sidecar branch path relations = %d, want 2: %#v", len(want), want)
	}
	if got := wirFacts.BranchPathRelations(point); !reflect.DeepEqual(got, want) {
		t.Fatalf("WIR compound branch path relations mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestLowerWithWIRCompoundBranchPathEvidenceMatchesSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local a, b, c = {}, {}, nil
if a == b and c ~= nil then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked bool
	for _, point := range built.Graph.RPO() {
		want := sidecarFacts.BranchPathEvidence(point)
		if len(want) == 0 {
			continue
		}
		checked = true
		if got := wirFacts.BranchPathEvidence(point); !reflect.DeepEqual(got, want) {
			t.Fatalf("WIR branch path evidence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
	if !checked {
		t.Fatal("test did not exercise branch path evidence")
	}
}

func TestLowerWithWIRFrozenTableBranchEvidenceMatchesSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = {}
local ok = true
if table.isfrozen(t) and ok then
    local hit = true
end
`, "table")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked bool
	for _, point := range built.Graph.RPO() {
		want := sidecarFacts.BranchPathEvidence(point)
		if len(want) == 0 {
			continue
		}
		checked = true
		if got := wirFacts.BranchPathEvidence(point); !reflect.DeepEqual(got, want) {
			t.Fatalf("WIR frozen-table branch path evidence at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
	if !checked {
		t.Fatal("test did not exercise frozen-table branch path evidence")
	}
}

func TestLowerWithWIRCompoundBranchRefinementLanesMatchSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local x: any = 1
local i: integer = 1
local xs: {string} = {}
if type(x) == "number" and i >= 1 and #xs > 0 then
    local hit = true
end
`, "type")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checkedRefinement, checkedLen, checkedNum bool
	for _, point := range built.Graph.RPO() {
		if want := sidecarFacts.BranchRefinements(point); len(want) != 0 {
			checkedRefinement = true
			if got := wirFacts.BranchRefinements(point); !reflect.DeepEqual(got, want) {
				t.Fatalf("WIR branch refinements at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
			}
		}
		if want := sidecarFacts.BranchLenRefinements(point); len(want) != 0 {
			checkedLen = true
			if got := wirFacts.BranchLenRefinements(point); !reflect.DeepEqual(got, want) {
				t.Fatalf("WIR branch len refinements at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
			}
		}
		if want := sidecarFacts.BranchNumFloorRefinements(point); len(want) != 0 {
			checkedNum = true
			if got := wirFacts.BranchNumFloorRefinements(point); !reflect.DeepEqual(got, want) {
				t.Fatalf("WIR branch num-floor refinements at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
			}
		}
	}
	if !checkedRefinement || !checkedLen || !checkedNum {
		t.Fatalf("test did not exercise all lanes: refinement=%v len=%v num=%v", checkedRefinement, checkedLen, checkedNum)
	}
}

func TestLowerWithWIRBranchPathRelationsDoesNotFallbackToSemanticCondition(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local a, b = {}, {}
if a == b then
    local hit = true
end
`)
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	if got := sidecarFacts.BranchPathRelations(point); len(got) == 0 {
		t.Fatalf("sidecar branch path relations = 0, test cannot prove WIR no-fallback")
	}

	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	if got := wirFacts.BranchPathRelations(point); len(got) != 0 {
		t.Fatalf("WIR branch path relations fell back to semantic condition: %#v", got)
	}
	if got := wirFacts.BranchPathEvidence(point); len(got) != 0 {
		t.Fatalf("WIR branch path evidence fell back to semantic condition: %#v", got)
	}
}

func TestLowerBranchDoesNotFallbackWhenWIRBranchInstructionMissing(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: string?): ()
    if x then local y = x end
end
`)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	if source, ok := facts.BranchConditionSource(point); ok {
		t.Fatalf("WIR mode branch at point %d fell back to semantic condition source: %#v", point, source)
	}
	if got := facts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic refinements: %#v", point, got)
	}
}

func TestLowerBranchDoesNotFallbackWhenWIRBranchCheckIsNone(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: string?): ()
    if x then local y = x end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	if source, ok := facts.BranchConditionSource(point); ok {
		t.Fatalf("WIR mode branch at point %d fell back to semantic condition source: %#v", point, source)
	}
	if got := facts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic refinements: %#v", point, got)
	}
	if got := facts.BranchLenRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic len refinements: %#v", point, got)
	}
	if got := facts.BranchNumFloorRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic num-floor refinements: %#v", point, got)
	}
	if got := facts.BranchPathRelations(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic path relations: %#v", point, got)
	}
	if got := facts.BranchPathEvidence(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic path evidence: %#v", point, got)
	}
}

func TestLowerTypeIsConditionDoesNotFallbackWhenWIRCallInstructionMissing(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Payload = { name: string }
local value: any = {}
if Payload:is(value) then local y = value end
`)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	point := requireStmtPoints(t, built, mustIfStmt(t, stmts, 2), 2)[1]
	if got := facts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode type-is condition at point %d fell back to semantic call/branch sidecars: %#v", point, got)
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

func TestLowerWithWIRBranchReachabilityMatchesSidecarLowering(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
if nil then local a = 1 end
if false then local b = 1 end
if true then local c = 1 end
if 0 then local d = 1 end
if "" then local e = 1 end
if not false then local f = 1 end
if true and false then local g = 1 end
if false or "fallback" then local h = 1 end
if false and f() then local h2 = 1 end
if true or f() then local h3 = 1 end
if {} then local i = 1 end
if (function() return 1 end) then local j = 1 end
if (nil :: any) then local k = 1 end
`, "f")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked int
	for _, point := range built.Graph.RPO() {
		fact, ok := result.BranchCondition(point)
		if !ok {
			continue
		}
		sidecarReachability, ok := semanticBranchEdgeReachability(fact.Condition)
		if !ok {
			continue
		}
		checked++
		for _, cond := range []bool{false, true} {
			want := sidecarReachability.EdgeUnreachable(cond)
			assertEqualBranchFacts(t, point, "branch reachability", want, wirFacts.BranchEdgeUnreachable(point, cond))
			assertEqualBranchFacts(t, point, "sidecar branch reachability", want, sidecarFacts.BranchEdgeUnreachable(point, cond))
		}
	}
	if checked != 17 {
		t.Fatalf("checked %d static branch conditions, want 17", checked)
	}
}

func TestWIRBranchReachabilityDoesNotFallbackToSidecarTruthiness(t *testing.T) {
	point := cfg.Point(1)
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	lowered := lowerer{wir: body}
	if got, ok := lowered.branchEdgeReachability(point, &ast.TrueExpr{}); ok {
		t.Fatalf("WIR branch reachability fell back to sidecar truthiness: %#v", got)
	}
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
	if source.HasExpr {
		source.ExprRef = 1
	}
	return branchConditionSourceCompare{source: source, ok: ok}
}

func assertWIRTruthyBranchConditionSource(t *testing.T, point cfg.Point, facts factflow.Facts, want path.Path) {
	t.Helper()
	source, ok := facts.BranchConditionSource(point)
	if !ok {
		t.Fatalf("missing WIR truthy branch condition source at point %d", point)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey != want.Key() || source.HasExpr {
		t.Fatalf("WIR truthy branch condition source at point %d = %#v, want path source %q", point, source, want.Key())
	}
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
