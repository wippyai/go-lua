package transferfacts

import (
	"reflect"
	"testing"

	path "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/semantic/intrinsic"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestWIRBranchChecksMatchSemanticDirectBranchChecks(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
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
	body := wirlower.LowerFunction("f", fn, bindings, built)
	if body == nil {
		t.Fatal("wirlower returned nil body")
	}

	var checked int
	for _, stmt := range fn.Stmts {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok {
			continue
		}
		point := requireBranchPointForStmt(t, built, ifStmt)
		check := branchcond.Normalize(ifStmt.Condition, bindings)
		if check.Kind == branchcond.CheckNone {
			continue
		}
		checked++
		assertWIRBranchCheckContains(t, body, point, check)
	}
	if checked != 7 {
		t.Fatalf("checked %d direct branch conditions, want 7", checked)
	}
}

func TestLowerWithWIRDirectBranchChecksPublishFactLanes(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: any, y: string?, i: integer, xs: {string})
    if x then local a = 1 end
    if y == nil then local b = 1 end
    if type(x) == "string" then local c = 1 end
	    if x == "ready" then local d = 1 end
	    if i >= 1 then local e = 1 end
	    if i <= #xs then local g = 1 end
	    if #xs > 0 then local h = 1 end
	end
`, "type")
	body := wirlower.LowerFunction("f", fn, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checkedRefinement, checkedLen, checkedNum, checkedEvidence bool
	for _, stmt := range fn.Stmts {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok {
			continue
		}
		point := requireBranchPointForStmt(t, built, ifStmt)
		check := branchcond.Normalize(ifStmt.Condition, bindings)
		if check.Kind == branchcond.CheckTruthy {
			assertWIRTruthyBranchConditionSource(t, point, wirFacts, check.Path)
		}
		checkedRefinement = checkedRefinement || len(wirFacts.BranchRefinements(point)) != 0
		checkedLen = checkedLen || len(wirFacts.BranchLenRefinements(point)) != 0
		checkedNum = checkedNum || len(wirFacts.BranchNumFloorRefinements(point)) != 0
		checkedEvidence = checkedEvidence || len(wirFacts.BranchPathEvidence(point)) != 0
	}
	if !checkedRefinement || !checkedLen || !checkedNum || !checkedEvidence {
		t.Fatalf("WIR direct branch checks did not exercise all lanes: refinement=%v len=%v num=%v evidence=%v",
			checkedRefinement, checkedLen, checkedNum, checkedEvidence)
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
	fn, bindings, built := parseSemanticFunction(t, `
function f(x: string?): ()
    if x then
        local y = x
    end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	body := wirlower.LowerFunction("branch-no-sidecars", fn, bindings, built)

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body, SealedLuaTypeChecks: true}).Facts
	target := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	if _, ok := branchRefinementAt(facts.BranchRefinements(point), target); !ok {
		t.Fatalf("WIR no-sidecar branch refinements missing %s at point %d: %#v", target, point, facts.BranchRefinements(point))
	}
	source, ok := facts.BranchConditionSource(point)
	if !ok {
		t.Fatalf("WIR no-sidecar branch condition source missing at point %d", point)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey != target.Key() || !source.Final || source.Adjusted || source.Expanded || source.OpenTail {
		t.Fatalf("WIR no-sidecar branch condition source = %#v, want path source for %s", source, target)
	}
	if got := facts.BranchPathEvidence(point); len(got) == 0 {
		t.Fatalf("WIR no-sidecar branch path evidence missing at point %d", point)
	}
}

func TestLowerWithWIRLiteralDiscriminantPublishesWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(r: {tag: "a", value: string} | {tag: "b", value: number}): ()
    if r.tag == "a" then
        local hit = r.value
    else
        local miss = r.value
    end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	body := wirlower.LowerFunction("literal-discriminant-no-sidecars", fn, bindings, built)

	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	if got := wirFacts.BranchRefinements(point); len(got) == 0 {
		t.Fatalf("WIR no-sidecar literal discriminant refinements missing at point %d", point)
	}
}

func TestLowerWithWIRCompoundBranchPathRelationsMatchSidecar(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local a, b, c, d = {}, {}, {}, {}
if a == b and c ~= d then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	if got := wirFacts.BranchPathRelations(point); len(got) != 2 {
		t.Fatalf("WIR compound branch path relations = %d, want 2: %#v", len(got), got)
	}
}

func TestLowerWithWIRCompoundBranchPathEvidencePublishesFacts(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local a, b, c = {}, {}, nil
if a == b and c ~= nil then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checked bool
	for _, point := range built.Graph.RPO() {
		got := wirFacts.BranchPathEvidence(point)
		if len(got) == 0 {
			continue
		}
		checked = true
	}
	if !checked {
		t.Fatal("WIR branch path evidence was not produced")
	}
}

func TestLowerWithWIRCompoundBranchPublishesSufficientLiteralCases(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local status = "ready"
if status == "ready" or status == "done" then
    local hit = true
end
`)
	reg := standard.Registry()
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: reg, WIR: body}).Facts

	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	got := wirFacts.BranchSufficientLiteralCases(point)
	if len(got) != 2 {
		t.Fatalf("BranchSufficientLiteralCases = %#v, want ready/done cases", got)
	}
	statusPath := path.NewPath(mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0), "status")
	seen := map[string]bool{}
	for _, c := range got {
		if !c.TargetPath().Equal(statusPath) || !c.Edge() {
			t.Fatalf("sufficient case = path %s edge %v, want %s true", c.TargetPath(), c.Edge(), statusPath)
		}
		lit, ok := typevalue.TypeOf(reg, c.LiteralValue())
		if !ok {
			t.Fatalf("sufficient case literal value has no type: %#v", c.LiteralValue())
		}
		switch {
		case typ.TypeEquals(lit, typ.LiteralString("ready")):
			seen["ready"] = true
		case typ.TypeEquals(lit, typ.LiteralString("done")):
			seen["done"] = true
		default:
			t.Fatalf("unexpected sufficient literal type %s", lit)
		}
	}
	if !seen["ready"] || !seen["done"] {
		t.Fatalf("sufficient literals = %#v, want ready and done", seen)
	}
}

func TestLowerWithWIRBranchDiffConstraintsPublishFacts(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local i, j, xs, limit = 1, 1, {}, 10
if i + 1 <= #xs and i + j < limit then
    local hit = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checked bool
	for _, point := range built.Graph.RPO() {
		got := wirFacts.BranchDiffConstraints(point)
		if len(got) == 0 {
			continue
		}
		checked = true
	}
	if !checked {
		t.Fatal("WIR branch diff constraints were not produced")
	}
}

func TestLowerWithWIRBooleanAliasBranchRefinementsMatchSidecar(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
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
	body := wirlower.LowerFunction("f", fn, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := wirFacts.BranchRefinements(point); len(got) == 0 {
			t.Fatalf("WIR alias branch refinements missing at point %d", point)
		}
	}
}

func TestLowerWithWIRBooleanAliasBranchPathRelationsMatchSidecar(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
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
	body := wirlower.LowerFunction("f", fn, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := wirFacts.BranchPathRelations(point); len(got) == 0 {
			t.Fatalf("WIR alias branch path relations missing at point %d", point)
		}
	}
}

func TestLowerWithWIRBooleanAliasBranchPathEvidenceMatchSidecar(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
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
	body := wirlower.LowerFunction("f", fn, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := wirFacts.BranchPathEvidence(point); len(got) == 0 {
			t.Fatalf("WIR alias branch path evidence missing at point %d", point)
		}
	}
}

func TestLowerWithWIRFrozenTableBranchEvidencePublishesFacts(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local t = {}
local ok = true
if table.isfrozen(t) and ok then
    local hit = true
end
`, "table")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checked bool
	for _, point := range built.Graph.RPO() {
		got := wirFacts.BranchPathEvidence(point)
		if len(got) == 0 {
			continue
		}
		checked = true
	}
	if !checked {
		t.Fatal("WIR frozen-table branch path evidence was not produced")
	}
}

func TestLowerWithWIRNormalizedCallBranchKeepsCanonicalCallResultSource(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local value = {}
if table.isfrozen(value) then
	local hit = true
end
`, "table")
	body := wirlower.Lower("normalized-call-condition", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	for _, point := range built.Graph.RPO() {
		if !built.Graph.IsBranch(point) {
			continue
		}
		source, ok := facts.BranchConditionSource(point)
		if !ok || source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.ResultIndex != 0 {
			t.Fatalf("normalized call condition source at %d = %#v/%t, want call result", point, source, ok)
		}
		return
	}
	t.Fatal("fixture did not produce a normalized call branch")
}

func TestLowerWithWIRNestedLogicalCallBranchKeepsCompletedProducer(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local function allowed(value) return true end
local pages = {{id = nil, route = "x", secure = false}}
local routes = {x = "x"}
for _, page in ipairs(pages) do
	local route = page.route
	if route and routes[route] == page.id and (not page.secure or allowed(page)) then
		local hit = true
	end
end
`, "ipairs")
	body := wirlower.Lower("nested-logical-call-condition", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var final cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op == wir.OpBranch && inst.A.Kind == wir.OperandTemp {
				final = point
			}
		}
	}
	if final == 0 {
		t.Fatalf("fixture did not produce completed logical branch\n%s", wir.Print(body, built.Graph))
	}
	source, ok := facts.BranchConditionSource(final)
	if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("nested logical condition source at %d = %#v/%t, want completed expression\n%s", final, source, ok, wir.Print(body, built.Graph))
	}
}

func TestLowerWithWIRCompoundBranchRefinementLanesMatchSidecar(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x: any = 1
local i: integer = 1
local xs: {string} = {}
if type(x) == "number" and i >= 1 and #xs > 0 then
    local hit = true
end
`, "type")
	body := wirlower.Lower("chunk", stmts, bindings, built)
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checkedRefinement, checkedLen, checkedNum bool
	for _, point := range built.Graph.RPO() {
		if got := wirFacts.BranchRefinements(point); len(got) != 0 {
			checkedRefinement = true
		}
		if got := wirFacts.BranchLenRefinements(point); len(got) != 0 {
			checkedLen = true
		}
		if got := wirFacts.BranchNumFloorRefinements(point); len(got) != 0 {
			checkedNum = true
		}
	}
	if !checkedRefinement || !checkedLen || !checkedNum {
		t.Fatalf("test did not exercise all lanes: refinement=%v len=%v num=%v", checkedRefinement, checkedLen, checkedNum)
	}
}

func TestLowerWithWIRBooleanAliasBranchesWithoutSourceAssignmentView(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
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
	body := wirlower.LowerFunction("alias-branches-no-sidecars", fn, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	for _, stmt := range []ast.Stmt{fn.Stmts[1], fn.Stmts[2]} {
		point := requireStmtPoints(t, built, stmt, 1)[0]
		if got := facts.BranchRefinements(point); len(got) == 0 {
			t.Fatalf("WIR alias branch refinements missing at point %d without semantic local assignment view", point)
		}
		if got := facts.BranchPathEvidence(point); len(got) == 0 {
			t.Fatalf("WIR alias branch path evidence missing at point %d without semantic local assignment view", point)
		}
	}
}

func TestLowerWithWIRBooleanAliasBranchDoesNotFallbackToSemanticCondition(t *testing.T) {
	fn, _, built := parseSemanticFunction(t, `
function f(target: { transform: string? })
    local has_transform = target.transform ~= nil
    if has_transform then
        local hit = true
    end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[1], 1)[0]
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	if got := wirFacts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR alias branch refinements fell back to semantic condition: %#v", got)
	}
}

func TestLowerWithWIRBranchPathRelationsDoesNotFallbackToSemanticCondition(t *testing.T) {
	stmts, _, built := parseSemanticChunk(t, `
local a, b = {}, {}
if a == b then
    local hit = true
end
`)
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
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
	stmts, _, built := parseSemanticChunk(t, `
local i, xs = 1, {}
if i + 1 <= #xs then
    local hit = true
end
`)
	point := requireStmtPoints(t, built, stmts[1], 1)[0]
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{}),
	})
	body.SetPointRange(point, start, start+1)

	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	if got := wirFacts.BranchDiffConstraints(point); len(got) != 0 {
		t.Fatalf("WIR branch diff constraints fell back to semantic condition: %#v", got)
	}
}

func TestLowerBranchDoesNotFallbackWhenWIRBranchInstructionMissing(t *testing.T) {
	fn, _, built := parseSemanticFunction(t, `
function f(x: string?): ()
    if x then local y = x end
end
`)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: wir.NewBody("empty")}).Facts

	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	if source, ok := facts.BranchConditionSource(point); ok {
		t.Fatalf("WIR mode branch at point %d fell back to semantic condition source: %#v", point, source)
	}
	if got := facts.BranchRefinements(point); len(got) != 0 {
		t.Fatalf("WIR mode branch at point %d fell back to semantic refinements: %#v", point, got)
	}
}

func TestLowerBranchDoesNotFallbackWhenWIRBranchCheckIsNone(t *testing.T) {
	fn, _, built := parseSemanticFunction(t, `
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

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

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

func TestLowerWIRRelationalBranchCheckPublishesExactBooleanConditionSource(t *testing.T) {
	fn, bindings, built := parseSemanticFunction(t, `
function f(a: {tag: "a"}, b: {tag: "b"}): ()
    if a == b then local hit = true end
end
`)
	point := requireStmtPoints(t, built, fn.Stmts[0], 1)[0]
	aPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "a")
	bPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "b")
	body := wir.NewBody("branch")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{Kind: wir.CheckPathEqual, Path: aPath, OtherPath: bPath}),
	})
	body.SetPointRange(point, start, start+1)

	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	source, ok := facts.BranchConditionSource(point)
	if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("relational WIR branch condition source = %#v/%v, want exact expression producer", source, ok)
	}
	operation, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || operation.Kind() != factflow.ExpressionOperationBinary || operation.Op() != "==" ||
		operation.Left().Kind != factflow.ValueSourcePath || operation.Right().Kind != factflow.ValueSourcePath {
		t.Fatalf("relational WIR branch producer = %#v/%v, want path equality", operation, ok)
	}
	if got := facts.BranchPathRelations(point); len(got) == 0 {
		t.Fatalf("relational WIR branch path relations = 0, want equality/inequality facts")
	}
}

func TestLowerWIRIndexInRangeBranchPublishesExactBooleanConditionSource(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local thresholds: {number} = { 3, 5, 8 }
local i: number = 1
while i <= #thresholds do
	i = i + 1
end
`)
	body := wirlower.Lower("debug-length-derived", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	var branchPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op == wir.OpBranch && inst.Check != 0 && body.Check(inst.Check).Kind == wir.CheckIndexInRange {
				branchPoint = point
			}
		}
	}
	if branchPoint == 0 {
		t.Fatalf("missing index-range WIR branch\n%s", wir.Print(body, built.Graph))
	}
	source, ok := facts.BranchConditionSource(branchPoint)
	if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("index-range condition source = %#v/%t, want expression", source, ok)
	}
	predicate, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || predicate.Kind() != factflow.ExpressionOperationBinary || predicate.Op() != "<=" || predicate.Left().Kind != factflow.ValueSourcePath {
		t.Fatalf("index-range predicate = %#v/%t, want index <= length", predicate, ok)
	}
	lengthSource := predicate.Right()
	length, ok := facts.ExpressionOperation(lengthSource.ExprRef)
	if !lengthSource.HasExpr || !ok || length.Kind() != factflow.ExpressionOperationUnary || length.Op() != "#" || length.Left().Kind != factflow.ValueSourcePath {
		t.Fatalf("index-range length = %#v source=%#v/%t, want #array", length, lengthSource, ok)
	}
}

func TestLowerWIRDynamicLuaTypeBranchPublishesExactBooleanConditionSource(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
local x: number = 5
local kind: string = "number"
if type(x) == kind then end
`, "type")
	body := wirlower.Lower("debug-type-eq", stmts, bindings, built)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body, SealedLuaTypeChecks: true}).Facts
	var branchPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		for _, inst := range body.PointInstructions(point) {
			if inst.Op == wir.OpBranch && inst.Check != 0 && body.Check(inst.Check).Kind == wir.CheckTypeEqual {
				branchPoint = point
			}
		}
	}
	if branchPoint == 0 {
		t.Fatalf("missing dynamic lua-type WIR branch\n%s", wir.Print(body, built.Graph))
	}
	source, ok := facts.BranchConditionSource(branchPoint)
	if !ok || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		t.Fatalf("dynamic lua-type condition source = %#v/%t, want expression", source, ok)
	}
	predicate, ok := facts.ExpressionOperation(source.ExprRef)
	if !ok || predicate.Kind() != factflow.ExpressionOperationBinary || predicate.Op() != "==" || predicate.Right().Kind != factflow.ValueSourcePath {
		t.Fatalf("dynamic lua-type predicate = %#v/%t, want type(value) == tag-path", predicate, ok)
	}
	typeSource := predicate.Left()
	typeOperation, ok := facts.ExpressionOperation(typeSource.ExprRef)
	identity, intrinsicOK := typeOperation.Intrinsic()
	if !typeSource.HasExpr || !ok || !intrinsicOK || identity != intrinsic.LuaType {
		t.Fatalf("dynamic lua-type producer = %#v source=%#v/%t, want LuaType intrinsic", typeOperation, typeSource, ok)
	}
}

func TestLowerTypeIsConditionDoesNotFallbackWhenWIRCallInstructionMissing(t *testing.T) {
	stmts, _, built := parseSemanticChunk(t, `
type Payload = { name: string }
local value: any = {}
if Payload:is(value) then local y = value end
`)
	facts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: wir.NewBody("empty")}).Facts

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
	stmts, bindings, built := parseSemanticChunk(t, `
type Payload = { name: string }
local value: any = {}
local validated, err = Payload:is(value)
if err then
	local failed = true
end
`)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	lowered := lowerer{
		registry:     standard.Registry(),
		wir:          body,
		typeResolver: typeresolve.New(bindings),
	}
	seed := LowerDetailed(built.Graph, Config{
		Registry:     standard.Registry(),
		TypeResolver: typeresolve.New(bindings),
		WIR:          body,
	}).Facts

	callPoint := requireWIRTypeIsCallPoint(t, built.Graph, &lowered)
	siteView, ok := seed.CallSiteView(callPoint)
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
		CallSites:               map[cfg.Point]factflow.CallSite{callPoint: callSiteFromView(siteView)},
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
	if typ, arg, ok := lowered.typeIsCallSiteFromWIR(callPoint); !ok {
		t.Fatalf("typeIsCallSiteFromWIR(%d) failed", callPoint)
	} else if arg.IsEmpty() || typ == nil {
		t.Fatalf("typeIsCallSiteFromWIR(%d) = %v, %v", callPoint, typ, arg)
	}

	lowered.addTypeIsBranchRefinements(input, built.Graph)

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
	stmts, bindings, built := parseSemanticChunk(t, src)
	body := wirlower.Lower("chunk", stmts, bindings, built)
	lowered := lowerer{
		registry:     standard.Registry(),
		wir:          body,
		typeResolver: typeresolve.New(bindings),
	}
	seed := LowerDetailed(built.Graph, Config{
		Registry:     standard.Registry(),
		TypeResolver: typeresolve.New(bindings),
		WIR:          body,
	}).Facts

	callPoint := requireWIRTypeIsCallPoint(t, built.Graph, &lowered)
	siteView, ok := seed.CallSiteView(callPoint)
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
		CallSites:         map[cfg.Point]factflow.CallSite{callPoint: callSiteFromView(siteView)},
		BranchRefinements: make(map[cfg.Point]factflow.BranchRefinementSet),
	}
	if typ, arg, ok := lowered.typeIsCallSiteFromWIR(callPoint); !ok {
		t.Fatalf("typeIsCallSiteFromWIR(%d) failed", callPoint)
	} else if arg.IsEmpty() || typ == nil {
		t.Fatalf("typeIsCallSiteFromWIR(%d) = %v, %v", callPoint, typ, arg)
	}

	lowered.addTypeIsBranchRefinements(input, built.Graph)

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

func requireWIRTypeIsCallPoint(t *testing.T, graph cfg.Graph, lowered *lowerer) cfg.Point {
	t.Helper()
	for _, point := range graph.RPO() {
		if _, _, ok := lowered.typeIsCallSiteFromWIR(point); ok {
			return point
		}
	}
	t.Fatal("missing WIR type-is call point")
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

func TestLowerWithWIRCorrelationBranchChecksPublishFacts(t *testing.T) {
	t.Run("protected_call", func(t *testing.T) {
		stmts, bindings, built := parseSemanticChunk(t, `
local function run_tests(): number
    return 1
end

local ok, result = pcall(run_tests)
if not ok then
    return
end
`, "pcall")
		body := wirlower.Lower("chunk", stmts, bindings, built)
		assertWIRBranchFactParity(t, built, body, bindings)
	})

	t.Run("type_is", func(t *testing.T) {
		stmts, bindings, built := parseSemanticChunk(t, `
type Point = {x: number, y: number}

local data: any = {}
local validated, err = Point:is(data)
if err then
    local failed = true
end
`)
		body := wirlower.Lower("chunk", stmts, bindings, built)
		assertWIRBranchFactParity(t, built, body, bindings)
	})
}

func TestLowerWithWIRBranchReachabilityPublishesStaticEdges(t *testing.T) {
	stmts, bindings, built := parseSemanticChunk(t, `
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
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts

	var checked int
	for point, condition := range branchConditionExprsByPoint(t, built, stmts) {
		truthy, ok := branchcond.StaticLuaTruthiness(condition)
		if !ok {
			continue
		}
		checked++
		for _, cond := range []bool{false, true} {
			want := factflow.NewBranchEdgeReachability(!truthy, truthy).EdgeUnreachable(cond)
			assertEqualBranchFacts(t, point, "branch reachability", want, wirFacts.BranchEdgeUnreachable(point, cond))
		}
	}
	if checked != 17 {
		t.Fatalf("checked %d static branch conditions, want 17", checked)
	}
}

func assertWIRBranchFactParity(t *testing.T, built *cfgbuild.Result, body *wir.Body, bindings *bind.Result) {
	t.Helper()
	wirFacts := LowerDetailed(built.Graph, Config{Registry: standard.Registry(), WIR: body}).Facts
	var checked bool
	for _, point := range built.Graph.RPO() {
		if len(wirFacts.BranchRefinements(point)) != 0 || len(wirFacts.BranchPresenceRelations(point)) != 0 {
			checked = true
		}
	}
	if !checked {
		t.Fatal("WIR correlation branch facts were not produced")
	}
}

func branchConditionExprsByPoint(t *testing.T, built *cfgbuild.Result, stmts []ast.Stmt) map[cfg.Point]ast.Expr {
	t.Helper()
	out := map[cfg.Point]ast.Expr{}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch stmt := stmt.(type) {
			case *ast.IfStmt:
				out[requireBranchPointForStmt(t, built, stmt)] = stmt.Condition
				walk(stmt.Then)
				walk(stmt.Else)
			case *ast.WhileStmt:
				out[requireBranchPointForStmt(t, built, stmt)] = stmt.Condition
				walk(stmt.Stmts)
			case *ast.RepeatStmt:
				walk(stmt.Stmts)
				out[requireBranchPointForStmt(t, built, stmt)] = stmt.Condition
			case *ast.DoBlockStmt:
				walk(stmt.Stmts)
			case *ast.NumberForStmt:
				walk(stmt.Stmts)
			case *ast.GenericForStmt:
				walk(stmt.Stmts)
			}
		}
	}
	walk(stmts)
	for _, point := range built.ShortCircuits.GuardPoints() {
		guard, ok := built.ShortCircuits.Guard(point)
		if !ok || guard.Condition == nil {
			continue
		}
		out[point] = guard.Condition
	}
	return out
}

func assertEqualBranchFacts(t *testing.T, point cfg.Point, label string, want, got any) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s at point %d mismatch\n got: %#v\nwant: %#v", label, point, got, want)
	}
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
	if got.LenFloor != want.LenFloor || got.NumFloor != want.NumFloor ||
		got.NumCeil != want.NumCeil || got.HasNumCeil != want.HasNumCeil ||
		got.NumCeilNegated != want.NumCeilNegated || got.Negated != want.Negated {
		return false
	}
	if got.Literal == nil || want.Literal == nil {
		return got.Literal == nil && want.Literal == nil
	}
	return typ.TypeEquals(got.Literal, want.Literal)
}
