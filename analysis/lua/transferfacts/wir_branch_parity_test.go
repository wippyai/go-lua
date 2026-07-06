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
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
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
		assertEqualBranchFacts(t, point, "diff constraints", sidecarFacts.BranchDiffConstraints(point), wirFacts.BranchDiffConstraints(point))
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

func TestLowerWithWIRDirectBranchPublishesWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(x: string?): ()
    if x ~= nil then
        local y = x
    end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	body := wirlower.Lower("branch-no-sidecars", fn.Stmts, bindings, built)

	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	target := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	if _, ok := branchRefinementAt(facts.BranchRefinements(point), target); !ok {
		t.Fatalf("WIR no-sidecar branch refinements missing %s at point %d: %#v", target, point, facts.BranchRefinements(point))
	}
	if got := facts.BranchPathEvidence(point); len(got) == 0 {
		t.Fatalf("WIR no-sidecar branch path evidence missing at point %d", point)
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

func TestLowerWithWIRBranchDiffConstraintsMatchesSidecar(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local i, j, xs, limit = 1, 1, {}, 10
if i + 1 <= #xs and i + j < limit then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	var checked bool
	for _, point := range built.Graph.RPO() {
		want := sidecarFacts.BranchDiffConstraints(point)
		if len(want) == 0 {
			continue
		}
		checked = true
		if got := wirFacts.BranchDiffConstraints(point); !reflect.DeepEqual(got, want) {
			t.Fatalf("WIR branch diff constraints at point %d mismatch\n got: %#v\nwant: %#v", point, got, want)
		}
	}
	if !checked {
		t.Fatal("test did not exercise branch diff constraints")
	}
}

func TestLowerWithWIRBooleanAliasBranchRefinementsMatchSidecar(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(target: { transform: string? })
    local has_transform = target.transform ~= nil
    if has_transform then
        local a = true
    end
    if not has_transform then
        local b = true
    end
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		assertEqualBranchFacts(t, point, "alias branch refinements", sidecarFacts.BranchRefinements(point), wirFacts.BranchRefinements(point))
	}
}

func TestLowerWithWIRBooleanAliasBranchPathRelationsMatchSidecar(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
    local same = a == b
    if same then
        local hit = true
    end
    if not same then
        local miss = true
    end
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		want := sidecarFacts.BranchPathRelations(point)
		if len(want) == 0 {
			t.Fatalf("sidecar alias branch path relations at point %d = 0", point)
		}
		assertEqualBranchFacts(t, point, "alias branch path relations", want, wirFacts.BranchPathRelations(point))
	}
}

func TestLowerWithWIRBooleanAliasBranchPathEvidenceMatchSidecar(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
    local same = a == b
    if same then
        local hit = true
    end
    if not same then
        local miss = true
    end
end
`)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		want := sidecarFacts.BranchPathEvidence(point)
		if len(want) == 0 {
			t.Fatalf("sidecar alias branch path evidence at point %d = 0", point)
		}
		assertEqualBranchFacts(t, point, "alias branch path evidence", want, wirFacts.BranchPathEvidence(point))
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

func TestLowerWithWIRBooleanAliasBranchDoesNotFallbackToSemanticCondition(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(target: { transform: string? })
    local has_transform = target.transform ~= nil
    if has_transform then
        local hit = true
    end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	targetPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "target").Field("transform")
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	if _, ok := branchRefinementAt(sidecarFacts.BranchRefinements(point), targetPath); !ok {
		t.Fatalf("sidecar branch refinements missing alias proof for %s: %#v", targetPath, sidecarFacts.BranchRefinements(point))
	}

	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	if got := wirFacts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR alias branch refinements fell back to semantic condition: %#v", got)
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
	if got := wirFacts.BranchDiffConstraints(point); len(got) != 0 {
		t.Fatalf("WIR branch diff constraints fell back to semantic condition: %#v", got)
	}
	if got := wirFacts.BranchPathEvidence(point); len(got) != 0 {
		t.Fatalf("WIR branch path evidence fell back to semantic condition: %#v", got)
	}
}

func TestLowerWithWIRBranchDiffConstraintsDoesNotFallbackToSemanticCondition(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local i, xs = 1, {}
if i + 1 <= #xs then
    local hit = true
end
`)
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	sidecarFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	if got := sidecarFacts.BranchDiffConstraints(point); len(got) == 0 {
		t.Fatalf("sidecar branch diff constraints = 0, test cannot prove WIR no-fallback")
	}

	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	if got := wirFacts.BranchDiffConstraints(point); len(got) != 0 {
		t.Fatalf("WIR branch diff constraints fell back to semantic condition: %#v", got)
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

func TestWIRTypeIsConditionBranchRefinementUsesLoweredCallSiteWithoutSemanticResult(t *testing.T) {
	assertWIRTypeIsConditionBranchRefinementWithoutSemanticResult(t, `
type Payload = { name: string }
local value: any = {}
if Payload:is(value) then local y = value end
`, true)
}

func TestWIRNegatedTypeIsConditionBranchRefinementUsesLoweredCallSiteWithoutSemanticResult(t *testing.T) {
	assertWIRTypeIsConditionBranchRefinementWithoutSemanticResult(t, `
type Payload = { name: string }
local value: any = {}
if not Payload:is(value) then local y = value end
`, false)
}

func TestWIRTypeIsResultCorrelationUsesLoweredCallSiteWithoutSemanticResult(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
type Payload = { name: string }
local value: any = {}
local validated, err = Payload:is(value)
if err then
	local failed = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	seed := Lower(result, built.Graph, Config{
		Registry:     standard.Registry(),
		Bindings:     bindings,
		TypeResolver: typeresolve.New(bindings),
		WIR:          body,
	})

	var callPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		view, ok := result.CallView(point)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		if _, _, ok := (&lowerer{bindings: bindings, typeResolver: typeresolve.New(bindings)}).typeIsCall(fact); ok {
			callPoint = point
			break
		}
	}
	if callPoint == 0 {
		t.Fatal("missing type-is call point")
	}
	site, ok := seed.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing lowered type-is callsite at point %d", callPoint)
	}
	assign := stmts[2].(*ast.LocalAssignStmt)
	validatedPath := path.NewPath(mustLocalAt(t, bindings, assign, 0), "validated")
	errPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "err")
	valueDecl := stmts[1].(*ast.LocalAssignStmt)
	valuePath := path.NewPath(mustLocalAt(t, bindings, valueDecl, 0), "value")
	branchPoint := requireWIRBranchPoint(t, built.Graph, body)
	input := &factflow.FactsInput{
		CallSites:               map[cfg.Point]factflow.CallSite{callPoint: site},
		RootAssignments:         make(map[cfg.Point]factflow.RootAssignment),
		BranchRefinements:       make(map[cfg.Point]factflow.BranchRefinementSet),
		BranchPresenceRelations: make(map[cfg.Point]factflow.BranchPresenceRelationSet),
	}
	for _, point := range built.Graph.RPO() {
		if assignment, ok := seed.RootAssignment(point); ok {
			input.RootAssignments[point] = assignment
		}
		if refinements := seed.BranchRefinements(point); len(refinements) != 0 {
			input.BranchRefinements[point] = factflow.NewBranchRefinementSet(refinements...)
		}
	}
	lowered := lowerer{
		registry:     standard.Registry(),
		bindings:     bindings,
		wir:          body,
		typeResolver: typeresolve.New(bindings),
	}
	if typ, arg, ok := lowered.typeIsCallSiteFromWIR(callPoint); !ok {
		t.Fatalf("typeIsCallSiteFromWIR(%d) failed", callPoint)
	} else if arg.IsEmpty() || typ == nil {
		t.Fatalf("typeIsCallSiteFromWIR(%d) = %v, %v", callPoint, typ, arg)
	}

	lowered.addTypeIsBranchRefinements(input, built.Graph, nil)

	refinement, ok := branchRefinementAt(input.BranchRefinements[branchPoint].Refinements(), valuePath)
	if !ok {
		t.Fatalf("missing WIR type-is result correlation refinement at point %d: %#v", branchPoint, input.BranchRefinements[branchPoint])
	}
	if _, ok := refinement.ValueForEdge(false); !ok {
		t.Fatalf("WIR type-is result correlation refinement = %#v, want false edge witness", refinement)
	}
	if !hasBranchPresenceRelation(input.BranchPresenceRelations[branchPoint].Relations(), errPath, validatedPath) {
		t.Fatalf("missing WIR type-is result presence correlation at point %d: %#v", branchPoint, input.BranchPresenceRelations[branchPoint])
	}
}

func assertWIRTypeIsConditionBranchRefinementWithoutSemanticResult(t *testing.T, src string, wantEdge bool) {
	t.Helper()
	stmts, bindings, built, result := parseSemanticChunk(t, src)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	seed := Lower(result, built.Graph, Config{
		Registry:     standard.Registry(),
		Bindings:     bindings,
		TypeResolver: typeresolve.New(bindings),
		WIR:          body,
	})

	var callPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		view, ok := result.CallView(point)
		if !ok {
			continue
		}
		fact, _ := view.Borrowed()
		if _, _, ok := (&lowerer{bindings: bindings, typeResolver: typeresolve.New(bindings)}).typeIsCall(fact); ok {
			callPoint = point
			break
		}
	}
	if callPoint == 0 {
		t.Fatal("missing type-is call point")
	}
	site, ok := seed.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing lowered callsite at type-is call point %d", callPoint)
	}
	branchPoint := requireWIRBranchPoint(t, built.Graph, body)
	valueDecl, ok := stmts[1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt[1] = %T, want local assignment", stmts[1])
	}
	valuePath := path.NewPath(mustLocalAt(t, bindings, valueDecl, 0), "value")
	input := &factflow.FactsInput{
		CallSites:         map[cfg.Point]factflow.CallSite{callPoint: site},
		BranchRefinements: make(map[cfg.Point]factflow.BranchRefinementSet),
	}
	lowered := lowerer{
		registry:     standard.Registry(),
		bindings:     bindings,
		wir:          body,
		typeResolver: typeresolve.New(bindings),
	}
	if typ, arg, ok := lowered.typeIsCallSiteFromWIR(callPoint); !ok {
		t.Fatalf("typeIsCallSiteFromWIR(%d) failed", callPoint)
	} else if arg.IsEmpty() || typ == nil {
		t.Fatalf("typeIsCallSiteFromWIR(%d) = %v, %v", callPoint, typ, arg)
	}

	lowered.addTypeIsBranchRefinements(input, built.Graph, nil)

	refinement, ok := branchRefinementAt(input.BranchRefinements[branchPoint].Refinements(), valuePath)
	if !ok {
		t.Fatalf("missing WIR type-is branch refinement at point %d: %#v", branchPoint, input.BranchRefinements[branchPoint])
	}
	if _, ok := refinement.ValueForEdge(wantEdge); !ok {
		t.Fatalf("WIR type-is refinement = %#v, want edge %v witness", refinement, wantEdge)
	}
}

func requireWIRBranchPoint(t *testing.T, graph cfg.Graph, body *wir.Body) cfg.Point {
	t.Helper()
	for _, point := range graph.RPO() {
		if body.HasInstruction(point, wir.OpBranch) {
			return point
		}
	}
	t.Fatal("missing WIR branch point")
	return 0
}

func hasBranchPresenceRelation(relations []factflow.BranchPresenceRelation, trigger, target path.Path) bool {
	for _, relation := range relations {
		if relation.TriggerPathRef().Equal(trigger) && relation.TargetPathRef().Equal(target) {
			return true
		}
	}
	return false
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
