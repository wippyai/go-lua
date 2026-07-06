package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/assertion"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/domain/value/variant"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/branchcond"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/module/manifest"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func TestLowerIdentifierNilTruthyFalsyBranches(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	nilRead := ident("x")
	nilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: nilRead, Rhs: &ast.NilExpr{}}}
	notNilRead := ident("x")
	notNilStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: notNilRead, Rhs: &ast.NilExpr{}}}
	truthyRead := ident("x")
	truthyStmt := &ast.IfStmt{Condition: truthyRead}
	falsyRead := ident("x")
	falsyStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: falsyRead}}
	stmts := []ast.Stmt{decl, nilStmt, notNilStmt, truthyStmt, falsyStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, nilRead), "x")
	nilPoint := requireStmtPoints(t, built, nilStmt, 1)[0]
	notNilPoint := requireStmtPoints(t, built, notNilStmt, 1)[0]
	truthyPoint := requireStmtPoints(t, built, truthyStmt, 1)[0]
	falsyPoint := requireStmtPoints(t, built, falsyStmt, 1)[0]
	assertLoweredBranchValuePresence(t, facts, nilPoint, xPath, presence.Absent(), true, presence.Present(), true)
	assertLoweredBranchPresenceProof(t, facts, nilPoint, xPath, presence.Absent(), true, false)
	assertLoweredBranchPresenceProof(t, facts, nilPoint, xPath, presence.Present(), false, true)
	assertLoweredBranchValuePresence(t, facts, notNilPoint, xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchValuePresence(t, facts, truthyPoint, xPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchFalsyAbsent(t, facts, truthyPoint, xPath, false)
	assertLoweredBranchPresenceProof(t, facts, truthyPoint, xPath, presence.Present(), true, false)
	assertLoweredBranchValuePresence(t, facts, falsyPoint, xPath, presence.Absent(), true, presence.Present(), true)
	assertLoweredBranchFalsyAbsent(t, facts, falsyPoint, xPath, true)
	assertLoweredBranchPresenceProof(t, facts, falsyPoint, xPath, presence.Present(), false, true)
}

func parseChunk(t *testing.T, src string) []ast.Stmt {
	t.Helper()
	stmts, err := parse.ParseString(src, "transferfacts_branch_test.lua")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	return stmts
}

func lowerFunctionFactsWithWIR(t *testing.T, name string, result *semantics.Result, built *cfgbuild.Result, bindings *bind.Result, reg *axis.Registry) factflow.Facts {
	t.Helper()
	fn := result.Function()
	if fn == nil {
		t.Fatal("semantic result is not a function")
	}
	body := wirlower.LowerFunction(name, fn, bindings, built)
	return Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
}

func lowerChunkFactsWithWIR(t *testing.T, name string, stmts []ast.Stmt, result *semantics.Result, built *cfgbuild.Result, bindings *bind.Result, reg *axis.Registry) factflow.Facts {
	t.Helper()
	body := wirlower.Lower(name, stmts, bindings, built)
	return Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
}

func TestLowerBooleanRootTruthyFalsyBranchesPublishLiteralRefinements(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(b: boolean)
	if b then local x = 1 end
	if not b then local y = 1 end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	truthyStmt := fn.Stmts[0].(*ast.IfStmt)
	falsyStmt := fn.Stmts[1].(*ast.IfStmt)
	bPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "b")
	truthyPoint := requireStmtPoints(t, built, truthyStmt, 1)[0]
	falsyPoint := requireStmtPoints(t, built, falsyStmt, 1)[0]

	assertBranchLiteralType(t, facts, truthyPoint, bPath, true, typ.True)
	assertBranchLiteralType(t, facts, truthyPoint, bPath, false, typ.False)
	assertBranchLiteralType(t, facts, falsyPoint, bPath, true, typ.False)
	assertBranchLiteralType(t, facts, falsyPoint, bPath, false, typ.True)
}

func TestTruthyBooleanRootImplicationUsesPolarityOnOuterEdge(t *testing.T) {
	lowered := &lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{1: typ.Boolean},
	}
	target := path.NewPath(symbol.ID(1), "b")
	refinements := lowered.truthyBooleanRootRefinementForImplication(
		branchcond.Check{Kind: branchcond.CheckFalsy, Path: target},
		false,
		false,
	)
	if len(refinements) != 1 {
		t.Fatalf("refinements = %#v, want one", refinements)
	}
	value, ok := refinements[0].ValueForEdge(false)
	if !ok {
		t.Fatalf("refinement missing false-edge value: %#v", refinements[0])
	}
	constraint, ok := value.Constraint()
	if !ok {
		t.Fatalf("false-edge value missing constraint: %#v", value)
	}
	got, ok := typevalue.TypeOf(standard.Registry(), constraint)
	if !ok || !typ.TypeEquals(got, typ.True) {
		t.Fatalf("false-edge literal = %v/%v, want true", got, ok)
	}
}

func TestLiteralBranchRefinementImpossibleStaticFieldBottomsImpossibleEdge(t *testing.T) {
	reg := standard.Registry()
	root := typetable.NewRecord().
		Field("ok", typ.True).
		Field("value", typ.String).
		Build()
	lowered := &lowerer{
		registry:    reg,
		symbolTypes: map[symbol.ID]typ.Type{1: root},
	}
	target := path.NewPath(symbol.ID(1), "x").Field("ok")

	equal, ok := lowered.literalBranchRefinement(target, branchcond.CheckLiteralEqual, typ.False)
	if !ok {
		t.Fatalf("missing impossible equality refinement")
	}
	if !equal.TargetPath().Equal(target.RootOnly()) {
		t.Fatalf("equality target = %s, want root %s", equal.TargetPath(), target.RootOnly())
	}
	assertBranchRefinementConstraint(t, reg, equal, true, product.Bottom(reg))
	if _, hasFalse := equal.ValueForEdge(false); hasFalse {
		t.Fatalf("equality false edge should not carry a bottom/no-op refinement: %#v", equal)
	}

	notEqual, ok := lowered.literalBranchRefinement(target, branchcond.CheckLiteralNot, typ.False)
	if !ok {
		t.Fatalf("missing impossible inequality refinement")
	}
	if !notEqual.TargetPath().Equal(target.RootOnly()) {
		t.Fatalf("inequality target = %s, want root %s", notEqual.TargetPath(), target.RootOnly())
	}
	assertBranchRefinementConstraint(t, reg, notEqual, false, product.Bottom(reg))
	if _, hasTrue := notEqual.ValueForEdge(true); hasTrue {
		t.Fatalf("inequality true edge should not carry a bottom/no-op refinement: %#v", notEqual)
	}
}

func TestLowerConditionalAssignmentPublishesValuePresenceImplicationAtMerge(t *testing.T) {
	_, bindings, built, result := parseSemanticFunction(t, `
function f(
    use_template: boolean,
    make_executor: () -> { with_context: (self: any, context: table) -> any }
)
    local executor: { with_context: (self: any, context: table) -> any }? = nil
    if not use_template then
        executor = make_executor()
    end

    if use_template then
        return
    else
        executor = executor:with_context({})
    end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	var found bool
	for _, point := range built.Graph.RPO() {
		for _, implication := range facts.PathValuePresenceImplications(point) {
			trigger := implication.TriggerPath()
			target := implication.TargetPath()
			if bindings.Name(trigger.Symbol) != "use_template" || bindings.Name(target.Symbol) != "executor" {
				continue
			}
			gotType, ok := typevalue.TypeOf(standard.Registry(), implication.TriggerValue())
			if !ok || !typ.TypeEquals(gotType, typ.False) {
				t.Fatalf("trigger value = %v/%v, want false literal", gotType, ok)
			}
			if implication.HasTargetValue() {
				if gotTargetType, ok := typevalue.TypeOf(standard.Registry(), implication.TargetValue()); !ok || gotTargetType == nil {
					t.Fatalf("target value type = %v/%v, want executor value witness", gotTargetType, ok)
				}
			} else if !presence.Equal(implication.TargetPresence(), presence.Present()) {
				t.Fatalf("target presence = %s, want present", implication.TargetPresence())
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing use_template=false => executor present implication")
	}
}

func TestLowerConditionalAssignmentPublishesValueRefinementImplicationAtMerge(t *testing.T) {
	_, bindings, built, result := parseSemanticFunction(t, `
function f(
    provided: any?,
    get_db: () -> { release: (self: any) -> () }?
)
    local db: any
    local need_release = false
    if provided then
        db = provided
    else
        db = get_db()
        need_release = true
    end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	var found bool
	for _, point := range built.Graph.RPO() {
		for _, implication := range facts.PathValuePresenceImplications(point) {
			trigger := implication.TriggerPath()
			target := implication.TargetPath()
			if bindings.Name(trigger.Symbol) != "need_release" || bindings.Name(target.Symbol) != "db" {
				continue
			}
			gotTriggerType, ok := typevalue.TypeOf(standard.Registry(), implication.TriggerValue())
			if !ok || !typ.TypeEquals(gotTriggerType, typ.True) {
				t.Fatalf("trigger value = %v/%v, want true literal", gotTriggerType, ok)
			}
			if !implication.HasTargetValue() {
				t.Fatalf("implication = %#v, want target value refinement", implication)
			}
			if gotTargetType, ok := typevalue.TypeOf(standard.Registry(), implication.TargetValue()); !ok || gotTargetType == nil {
				t.Fatalf("target value type = %v/%v, want provider return type witness", gotTargetType, ok)
			}
			found = true
		}
	}
	if !found {
		var got []factflow.PathValuePresenceImplication
		for _, point := range built.Graph.RPO() {
			got = append(got, facts.PathValuePresenceImplications(point)...)
		}
		t.Fatalf("missing need_release=true => db has provider return value implication; got %#v", got)
	}
}

func TestLowerConditionalAssignmentPublishesValueRefinementThroughErrorGuard(t *testing.T) {
	_, bindings, built, result := parseSemanticFunction(t, `
function f(
    provided: any?,
    get_db: () -> ({ release: (self: any) -> () }?, string?)
)
    local db: any
    local db_err: any
    local need_release = false
    if provided then
        db = provided
    else
        db, db_err = get_db()
        if db_err then
            return
        end
        need_release = true
    end
    if need_release then
        db:release()
    end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	var found bool
	for _, point := range built.Graph.RPO() {
		for _, implication := range facts.PathValuePresenceImplications(point) {
			trigger := implication.TriggerPath()
			target := implication.TargetPath()
			if bindings.Name(trigger.Symbol) != "need_release" || bindings.Name(target.Symbol) != "db" {
				continue
			}
			gotTriggerType, ok := typevalue.TypeOf(standard.Registry(), implication.TriggerValue())
			if !ok || !typ.TypeEquals(gotTriggerType, typ.True) {
				t.Fatalf("trigger value = %v/%v, want true literal", gotTriggerType, ok)
			}
			if !implication.HasTargetValue() {
				t.Fatalf("implication = %#v, want target value refinement", implication)
			}
			if gotTargetType, ok := typevalue.TypeOf(standard.Registry(), implication.TargetValue()); !ok || gotTargetType == nil {
				t.Fatalf("target value type = %v/%v, want provider return type witness", gotTargetType, ok)
			}
			found = true
		}
	}
	if !found {
		var got []factflow.PathValuePresenceImplication
		for _, point := range built.Graph.RPO() {
			got = append(got, facts.PathValuePresenceImplications(point)...)
		}
		t.Fatalf("missing need_release=true => db has provider return value implication after error guard; got %#v", got)
	}
}

func TestLowerConditionalAssignmentUsesCapturedRequireExportReturn(t *testing.T) {
	stmts := parseChunk(t, `
local sql = require("sql")

function run(options: { db: any?, database_id: string? }): ()
    local db: any
    local db_err: any
    local need_release = false
    if options.db then
        db = options.db
    else
        db, db_err = sql.get(tostring(options.database_id))
        if db_err then
            return
        end
        need_release = true
    end
    if need_release then
        db:release()
    end
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	def, ok := stmts[1].(*ast.FuncDefStmt)
	if !ok || def.Func == nil {
		t.Fatalf("stmt[1] = %T, want function definition", stmts[1])
	}
	built := cfgbuild.BuildFunction(def.Func, bindings)
	result, err := semantics.ExtractFunction(def.Func, bindings, built)
	if err != nil {
		t.Fatalf("ExtractFunction: %v", err)
	}
	dbType := typetable.NewRecord().
		Field("release", typ.Func().Param("self", typ.Self).Build()).
		Build()
	getType := typ.Func().
		Param("name", typ.String).
		Returns(typeexpr.Optional(dbType), typeexpr.Optional(typ.String)).
		Build()
	sqlManifest := manifest.New("sql")
	sqlManifest.SetExport(typetable.NewRecord().Field("get", getType).Build())
	lowered := LowerWithSidecars(result, built.Graph, Config{
		Registry:      standard.Registry(),
		Bindings:      bindings,
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{sqlManifest}},
	})
	assertNeedReleaseImpliesDBValue(t, bindings, built.Graph, lowered.Facts)

	body := wirlower.Lower("run", def.Func.Stmts, bindings, built)
	wirFacts := Lower(result, built.Graph, Config{
		Registry:      standard.Registry(),
		Bindings:      bindings,
		ModuleExports: importlookup.Source{Manifests: []*manifest.Manifest{sqlManifest}},
		WIR:           body,
	})
	assertNeedReleaseImpliesDBValue(t, bindings, built.Graph, wirFacts)
}

func TestConditionalAssignmentCallSourceValueUsesLoweredCallSite(t *testing.T) {
	reg := standard.Registry()
	callPoint := cfg.Point(1200)
	providerSym := symbol.ID(1201)
	dbType := typetable.NewRecord().
		Field("release", typ.Func().Param("self", typ.Self).Build()).
		Build()
	providerType := typ.Func().Returns(dbType).Build()
	source, ok := factflow.NewCallValueSource(0, 0, 0, 0, callPoint, factflow.ValueSourceShape{
		Final: true,
	})
	if !ok {
		t.Fatal("failed to construct call value source")
	}
	input := &factflow.FactsInput{
		CallSites: map[cfg.Point]factflow.CallSite{
			callPoint: factflow.NewCallSite(factflow.CallSiteConfig{
				Context:      factflow.CallSiteContextAssignmentSource,
				CalleeSymbol: providerSym,
				CalleePath:   path.NewPath(providerSym, "make_db"),
			}),
		},
	}
	lowered := lowerer{
		registry: reg,
		symbolTypes: map[symbol.ID]typ.Type{
			providerSym: providerType,
		},
		typeValues: typevalue.NewCache(),
	}

	value, ok := lowered.rootAssignmentSourceValue(input, source)
	if !ok {
		t.Fatalf("missing call source value from lowered call site")
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, dbType) {
		t.Fatalf("call source value type = %v/%v, want %v", got, ok, dbType)
	}
}

func assertNeedReleaseImpliesDBValue(t *testing.T, bindings *bind.Result, graph cfg.Graph, facts factflow.Facts) {
	t.Helper()
	var found bool
	for _, point := range graph.RPO() {
		for _, implication := range facts.PathValuePresenceImplications(point) {
			trigger := implication.TriggerPath()
			target := implication.TargetPath()
			if bindings.Name(trigger.Symbol) != "need_release" || bindings.Name(target.Symbol) != "db" {
				continue
			}
			if !implication.HasTargetValue() {
				t.Fatalf("implication = %#v, want target value from captured require return", implication)
			}
			found = true
		}
	}
	if !found {
		var got []factflow.PathValuePresenceImplication
		for _, point := range graph.RPO() {
			got = append(got, facts.PathValuePresenceImplications(point)...)
		}
		t.Fatalf("missing need_release=true => db has sql.get return value implication; got %#v", got)
	}
}

func TestLowerConditionalAssignmentPublishesValuePresenceImplicationBeforeLoop(t *testing.T) {
	_, bindings, built, result := parseSemanticFunction(t, `
function f(
    use_template: boolean,
    make_executor: () -> { with_context: (self: any, context: table) -> any }
)
    local executor: { with_context: (self: any, context: table) -> any }? = nil
    if not use_template then
        executor = make_executor()
    end

    for i = 1, 2 do
        if use_template then
        else
            executor = executor:with_context({})
        end
    end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	assertUseTemplateFalseImpliesExecutorPresent(t, bindings, built, facts)
}

func TestLowerStaticTruthinessMarksImpossibleBranchEdges(t *testing.T) {
	tests := []struct {
		name             string
		stmt             ast.Stmt
		wantTrueBlocked  bool
		wantFalseBlocked bool
	}{
		{
			name:             "while true",
			stmt:             &ast.WhileStmt{Condition: &ast.TrueExpr{}},
			wantFalseBlocked: true,
		},
		{
			name:            "if false",
			stmt:            &ast.IfStmt{Condition: &ast.FalseExpr{}},
			wantTrueBlocked: true,
		},
		{
			name: "repeat until nil",
			stmt: &ast.RepeatStmt{
				Condition: &ast.NilExpr{},
				Stmts:     []ast.Stmt{localAssign([]string{"x"}, number("1"))},
			},
			wantTrueBlocked: true,
		},
		{
			name: "logical constant",
			stmt: &ast.IfStmt{Condition: &ast.LogicalOpExpr{
				Operator: "or",
				Lhs:      &ast.TrueExpr{},
				Rhs:      ident("dynamic"),
			}},
			wantFalseBlocked: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stmts := []ast.Stmt{tc.stmt}
			bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"dynamic"}})
			built := cfgbuild.BuildChunk(stmts, bindings)
			_, err := semantics.ExtractChunk(stmts, bindings, built)
			if err != nil {
				t.Fatalf("ExtractChunk: %v", err)
			}

			body := wirlower.Lower("static-truthiness", stmts, bindings, built)
			facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
			point := requireStmtPoints(t, built, tc.stmt, 1)[0]
			if got := facts.BranchEdgeUnreachable(point, true); got != tc.wantTrueBlocked {
				t.Fatalf("true-edge unreachable = %v, want %v", got, tc.wantTrueBlocked)
			}
			if got := facts.BranchEdgeUnreachable(point, false); got != tc.wantFalseBlocked {
				t.Fatalf("false-edge unreachable = %v, want %v", got, tc.wantFalseBlocked)
			}
		})
	}
}

func assertUseTemplateFalseImpliesExecutorPresent(t *testing.T, bindings *bind.Result, built *cfgbuild.Result, facts factflow.Facts) {
	t.Helper()
	var found bool
	for _, point := range built.Graph.RPO() {
		for _, implication := range facts.PathValuePresenceImplications(point) {
			trigger := implication.TriggerPath()
			target := implication.TargetPath()
			if bindings.Name(trigger.Symbol) != "use_template" || bindings.Name(target.Symbol) != "executor" {
				continue
			}
			gotType, ok := typevalue.TypeOf(standard.Registry(), implication.TriggerValue())
			if !ok || !typ.TypeEquals(gotType, typ.False) {
				t.Fatalf("trigger value = %v/%v, want false literal", gotType, ok)
			}
			if implication.HasTargetValue() {
				if gotTargetType, ok := typevalue.TypeOf(standard.Registry(), implication.TargetValue()); !ok || gotTargetType == nil {
					t.Fatalf("target value type = %v/%v, want executor value witness", gotTargetType, ok)
				}
			} else if !presence.Equal(implication.TargetPresence(), presence.Present()) {
				t.Fatalf("target presence = %s, want present", implication.TargetPresence())
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("missing use_template=false => executor present implication")
	}
}

func TestLowerCompoundOrLiteralImplicationsStayOnProvenOuterEdges(t *testing.T) {
	_, bindings, built, result := parseSemanticFunction(t, `
function f(kind: string?)
	if not kind or kind == "auto" or kind == "any" or kind == "" then
		return { mode = "AUTO" }
	end
	return nil
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	var directFalsyPoint cfg.Point
	var compoundPoint cfg.Point
	for _, point := range built.Graph.RPO() {
		fact, ok := result.BranchCondition(point)
		if !ok {
			continue
		}
		if fact.Check.Kind == branchcond.CheckFalsy {
			directFalsyPoint = point
		}
		if fact.Check.Kind == branchcond.CheckNone {
			if _, ok := fact.Condition.(*ast.LogicalOpExpr); ok && compoundPoint == 0 {
				compoundPoint = point
			}
		}
	}
	if directFalsyPoint == 0 {
		t.Fatal("missing direct `not kind` branch point")
	}
	if compoundPoint == 0 {
		t.Fatal("missing compound `or` branch point")
	}
	assertTruthyEvidenceOpposite(t, facts, directFalsyPoint, true)
	assertTruthyEvidenceOpposite(t, facts, compoundPoint, false)
	for _, refinement := range facts.BranchRefinements(compoundPoint) {
		if hasBranchRefinementValue(refinement, true) {
			t.Fatalf("compound branch implication unexpectedly refines true edge for %s", refinement.TargetPath())
		}
	}
}

func TestBranchEdgeAndImplicationRefinementsShareSingleEdgePlacement(t *testing.T) {
	lowered := &lowerer{registry: standard.Registry()}
	target := path.NewPath(symbol.ID(1), "x")
	check := branchcond.Check{Kind: branchcond.CheckNotNil, Path: target}

	edgeRefinement, ok := lowered.branchEdgeRefinement(check, false)
	if !ok {
		t.Fatal("branchEdgeRefinement returned false")
	}
	impliedRefinement, ok := lowered.branchImplicationRefinement(branchcond.ImpliedCheck{
		Check:    check,
		Polarity: false,
		Edge:     false,
	})
	if !ok {
		t.Fatal("branchImplicationRefinement returned false")
	}
	if !edgeRefinement.TargetPath().Equal(target) || !impliedRefinement.TargetPath().Equal(target) {
		t.Fatalf("targets = %s / %s, want %s", edgeRefinement.TargetPath(), impliedRefinement.TargetPath(), target)
	}
	if _, ok := edgeRefinement.ValueForEdge(true); ok {
		t.Fatalf("direct refinement unexpectedly populated true edge: %#v", edgeRefinement)
	}
	if _, ok := impliedRefinement.ValueForEdge(true); ok {
		t.Fatalf("implied refinement unexpectedly populated true edge: %#v", impliedRefinement)
	}
	directValue, ok := edgeRefinement.ValueForEdge(false)
	if !ok {
		t.Fatalf("direct refinement missing false-edge value: %#v", edgeRefinement)
	}
	impliedValue, ok := impliedRefinement.ValueForEdge(false)
	if !ok {
		t.Fatalf("implied refinement missing false-edge value: %#v", impliedRefinement)
	}
	if !directValue.HasPresence(presence.Absent()) || !impliedValue.HasPresence(presence.Absent()) {
		t.Fatalf("false-edge values differ\n direct: %#v\nimplied: %#v", directValue, impliedValue)
	}
}

func hasBranchRefinementValue(refinement factflow.BranchRefinement, edge bool) bool {
	_, ok := refinement.ValueForEdge(edge)
	return ok
}

func assertTruthyEvidenceOpposite(t *testing.T, facts factflow.Facts, point cfg.Point, want bool) {
	t.Helper()
	for _, proof := range facts.BranchPathEvidence(point) {
		if proof.Kind() != factflow.BranchPathEvidenceTruthy {
			continue
		}
		if got := proof.OppositeEdgeImpliesFalsy(); got != want {
			t.Fatalf("point %d truthy evidence opposite-falsy = %v, want %v", point, got, want)
		}
		return
	}
	t.Fatalf("point %d has no truthy evidence", point)
}

func TestLowerMemberPathBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	rootRead := ident("t")
	memberStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "~=", Lhs: dot(rootRead, "child"), Rhs: &ast.NilExpr{}}}
	stmts := []ast.Stmt{decl, memberStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, rootRead), "t").Field("child")
	assertLoweredBranchValuePresence(t, facts, requireStmtPoints(t, built, memberStmt, 1)[0], wantPath, presence.Present(), true, presence.Absent(), true)
}

func TestLowerMemberPathTruthyBranchEvidence(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	rootRead := ident("t")
	memberStmt := &ast.IfStmt{Condition: dot(rootRead, "child")}
	stmts := []ast.Stmt{decl, memberStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, rootRead), "t").Field("child")
	point := requireStmtPoints(t, built, memberStmt, 1)[0]
	assertLoweredBranchValuePresence(t, facts, point, wantPath, presence.Present(), true, presence.Absent(), true)
	assertLoweredBranchPresenceProof(t, facts, point, wantPath, presence.Present(), true, false)
	assertLoweredBranchTruthyProof(t, facts, point, wantPath, true, false)
}

func TestLowerTypeGuardBranchPathEvidence(t *testing.T) {
	decl := localAssign([]string{"x"}, ident("input"))
	typeRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(typeRead),
		Rhs:      stringLit("string"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, typeRead), "x")
	assertLoweredBranchPresenceProof(t, facts, point, xPath, presence.Present(), true, false)
}

func TestLowerLogicalAndBranchPublishesTrueEdgeConjunctRefinements(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local data_func = raw
if data_func and data_func ~= "" then
end
`, "raw")

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	dataFunc := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	ifStmt := mustIfStmt(t, stmts, 1)
	assertLoweredBranchValuePresence(
		t,
		facts,
		requireStmtPoints(t, built, ifStmt, 1)[0],
		path.NewPath(dataFunc, "data_func"),
		presence.Present(), true,
		presence.Bottom(), false,
	)
}

func TestLowerLogicalOrBranchPublishesFalseEdgeDisjunctRefinements(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local page = input
if not page or not page.data_func or page.data_func == "" then
end
`, "input")

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	page := mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0)
	ifStmt := mustIfStmt(t, stmts, 1)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	assertLoweredBranchValuePresence(
		t,
		facts,
		point,
		path.NewPath(page, "page"),
		presence.Bottom(), false,
		presence.Present(), true,
	)
	assertLoweredBranchValuePresence(
		t,
		facts,
		point,
		path.NewPath(page, "page").Field("data_func"),
		presence.Bottom(), false,
		presence.Present(), true,
	)
	assertRootRefinementsBeforeDescendants(t, facts.BranchRefinements(point))
}

func TestLowerProtectedCallSuccessGuardRefinesPayloadToCallbackReturn(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local function run_tests(): number
	return 1
end

local ok, result = pcall(run_tests)
if not ok then
	return
end
`, "pcall")

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, reg)
	assign := mustLocalStmt(t, stmts, 1)
	payloadPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "result")
	ifStmt := mustIfStmt(t, stmts, 2)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), payloadPath)
	if !ok {
		t.Fatalf("missing pcall payload refinement at point %d; got %#v", point, facts.BranchRefinements(point))
	}
	value, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge pcall payload refinement: %#v", refinement)
	}
	constraint, ok := value.Constraint()
	if !ok {
		t.Fatalf("pcall payload refinement has no constraint: %#v", value)
	}
	got, ok := typevalue.TypeOf(reg, constraint)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("pcall payload refinement type = %v/%v, want number; value=%#v", got, ok, constraint)
	}
}

func TestLowerProtectedCallPayloadTypeComesFromWIRCallbackPath(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local function run_tests(): string
	return "wrong"
end

local function other_tests(): number
	return 1
end

local ok, result = pcall(run_tests)
if not ok then
	return
end
`, "pcall")

	runStmt := mustLocalStmt(t, stmts, 0)
	otherStmt := mustLocalStmt(t, stmts, 1)
	assign := mustLocalStmt(t, stmts, 2)
	points := requireStmtPoints(t, built, assign, 3)
	callPoint, okAssignPoint, payloadAssignPoint := points[0], points[1], points[2]
	okPath := path.NewPath(mustLocalAt(t, bindings, assign, 0), "ok")
	payloadPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "result")
	runPath := path.NewPath(mustLocalAt(t, bindings, runStmt, 0), "run_tests")
	otherPath := path.NewPath(mustLocalAt(t, bindings, otherStmt, 0), "other_tests")
	pcallSym, ok := bindings.GlobalSymbol("pcall")
	if !ok {
		t.Fatal("missing pcall global symbol")
	}
	pcallPath := path.NewPath(pcallSym, "pcall")
	ifStmt := mustIfStmt(t, stmts, 3)
	branchPoint := requireStmtPoints(t, built, ifStmt, 1)[0]

	body := wir.NewBody("pcall-callback-owner")
	okTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	payloadTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 2}
	callStart := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(pcallPath))}},
		List:    body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
		Results: body.AppendOperands([]wir.Operand{okTemp, payloadTemp}),
	})
	body.SetPointRange(callPoint, callStart, callStart+1)
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       0,
		ResultIndex: 0,
		Path:        okPath,
	})
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetLocalAssignment,
		Index:       1,
		ResultIndex: 1,
		Path:        payloadPath,
	})
	okAssignStart := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: okAssignPoint,
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(okPath))},
		A:     okTemp,
	})
	body.SetPointRange(okAssignPoint, okAssignStart, okAssignStart+1)
	payloadAssignStart := body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: payloadAssignPoint,
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadPath))},
		A:     payloadTemp,
	})
	body.SetPointRange(payloadAssignPoint, payloadAssignStart, payloadAssignStart+1)
	branchStart := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: branchPoint,
		Check: body.InternCheck(wir.Check{Kind: wir.CheckFalsy, Path: okPath}),
	})
	body.SetPointRange(branchPoint, branchStart, branchStart+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	refinement, ok := branchRefinementAt(facts.BranchRefinements(branchPoint), payloadPath)
	if !ok {
		t.Fatalf("missing pcall payload refinement at point %d; got %#v", branchPoint, facts.BranchRefinements(branchPoint))
	}
	value, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge pcall payload refinement: %#v", refinement)
	}
	constraint, ok := value.Constraint()
	if !ok {
		t.Fatalf("pcall payload refinement has no constraint: %#v", value)
	}
	got, ok := typevalue.TypeOf(reg, constraint)
	if !ok || !typ.TypeEquals(got, typ.Number) || typ.TypeEquals(got, typ.String) {
		t.Fatalf("pcall payload refinement type = %v/%v, want WIR callback %v not semantic callback %v; value=%#v", got, ok, otherPath, runPath, constraint)
	}
}

func TestLowerProtectedCallSuccessGuardInWIRModeDoesNotFallbackToSidecar(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local function run_tests(): number
	return 1
end

local ok, result = pcall(run_tests)
if not ok then
	return
end
`, "pcall")

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: wir.NewBody("empty")})
	assign := mustLocalStmt(t, stmts, 1)
	payloadPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "result")
	ifStmt := mustIfStmt(t, stmts, 2)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	if refinement, ok := branchRefinementAt(facts.BranchRefinements(point), payloadPath); ok {
		t.Fatalf("WIR mode protected-call refinement fell back to semantic branch check at point %d: %#v", point, refinement)
	}
}

func TestLowerProtectedCallSuccessGuardUsesWIRCallSiteWithoutSemanticResult(t *testing.T) {
	reg := standard.Registry()
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
	seed := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	assign := mustLocalStmt(t, stmts, 1)
	callPoint := requireStmtPoints(t, built, assign, 3)[0]
	site, ok := seed.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing lowered pcall callsite at point %d", callPoint)
	}
	payloadPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "result")
	ifStmt := mustIfStmt(t, stmts, 2)
	branchPoint := requireStmtPoints(t, built, ifStmt, 1)[0]
	input := &factflow.FactsInput{
		CallSites:         map[cfg.Point]factflow.CallSite{callPoint: site},
		RootAssignments:   make(map[cfg.Point]factflow.RootAssignment),
		BranchRefinements: make(map[cfg.Point]factflow.BranchRefinementSet),
	}
	for _, point := range built.Graph.RPO() {
		if assignment, ok := seed.RootAssignment(point); ok {
			input.RootAssignments[point] = assignment
		}
	}
	lowered := lowerer{
		registry:    reg,
		bindings:    bindings,
		wir:         body,
		symbolTypes: lowerSymbolTypes(bindings, built.Graph, built.Meta, result, nil, importlookup.Source{}),
	}

	lowered.addProtectedCallBranchRefinements(input, built.Graph)

	refinement, ok := branchRefinementAt(input.BranchRefinements[branchPoint].Refinements(), payloadPath)
	if !ok {
		t.Fatalf("missing WIR pcall payload refinement at point %d; got %#v", branchPoint, input.BranchRefinements[branchPoint])
	}
	value, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge WIR pcall payload refinement: %#v", refinement)
	}
	constraint, ok := value.Constraint()
	if !ok {
		t.Fatalf("WIR pcall payload refinement has no constraint: %#v", value)
	}
	got, ok := typevalue.TypeOf(reg, constraint)
	if !ok || !typ.TypeEquals(got, typ.Number) {
		t.Fatalf("WIR pcall payload refinement type = %v/%v, want number; value=%#v", got, ok, constraint)
	}
}

func TestLowerProtectedCallDoesNotFallbackWhenWIRCallInstructionMissing(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local function run_tests(): number
	return 1
end

local ok, result = pcall(run_tests)
if not ok then
	return
end
`, "pcall")

	assign := mustLocalStmt(t, stmts, 1)
	okPath := path.NewPath(mustLocalAt(t, bindings, assign, 0), "ok")
	payloadPath := path.NewPath(mustLocalAt(t, bindings, assign, 1), "result")
	ifStmt := mustIfStmt(t, stmts, 2)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	body := wir.NewBody("branch-only")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpBranch,
		Point: point,
		Check: body.InternCheck(wir.Check{Kind: wir.CheckFalsy, Path: okPath}),
	})
	body.SetPointRange(point, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})
	if refinement, ok := branchRefinementAt(facts.BranchRefinements(point), payloadPath); ok {
		t.Fatalf("WIR mode protected-call refinement used semantic call at point %d without WIR call instruction: %#v", point, refinement)
	}
}

func TestLowerNegatedConjunctionIndexGuardPublishesFalseEdgeProofs(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(xs: {number}, i: number)
	if not (i >= 1 and i <= #xs) then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	xs := bindings.ParamSlots(fn)[0].Symbol
	i := bindings.ParamSlots(fn)[1].Symbol
	xsPath := path.NewPath(xs, "xs")
	iPath := path.NewPath(i, "i")
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	branchFact, _ := result.BranchCondition(point)

	foundFloor := false
	for _, floor := range facts.BranchNumFloorRefinements(point) {
		if floor.TargetPath().Equal(iPath) && floor.Floor() == 1 && !floor.Cond() {
			foundFloor = true
			break
		}
	}
	if !foundFloor {
		t.Fatalf("missing false-edge i >= 1 floor; got %#v; branch check=%#v condition=%T %#v",
			facts.BranchNumFloorRefinements(point), branchFact.Check, branchFact.Condition, branchFact.Condition)
	}

	foundRange := false
	for _, proof := range facts.BranchPathEvidence(point) {
		other, hasOther := proof.OtherPath()
		if proof.Kind() == factflow.BranchPathEvidenceIndexInRange &&
			proof.Path().Equal(iPath) &&
			hasOther && other.Equal(xsPath) &&
			proof.ActiveOnEdge(false) {
			foundRange = true
			break
		}
	}
	if !foundRange {
		t.Fatalf("missing false-edge i <= #xs evidence; got %#v", facts.BranchPathEvidence(point))
	}
}

func TestLowerLengthNotEqualGuardPublishesFalseEdgeFloor(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(parts: {string})
	if #parts ~= 2 then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	parts := bindings.ParamSlots(fn)[0].Symbol
	partsPath := path.NewPath(parts, "parts")
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	for _, floor := range facts.BranchLenRefinements(point) {
		if floor.ArrayPath().Equal(partsPath) && floor.Floor() == 2 && !floor.Cond() {
			return
		}
	}
	t.Fatalf("missing false-edge #parts ~= 2 length floor; got %#v", facts.BranchLenRefinements(point))
}

func TestLowerNegatedConjunctionNilChecksPublishFalseEdgeRefinements(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: string?, y: string?)
	if not (x == nil and y == nil) then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	yPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "y")
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	for _, want := range []path.Path{xPath, yPath} {
		refinement, ok := branchRefinementAt(facts.BranchRefinements(point), want)
		if !ok {
			t.Fatalf("missing branch refinement for %s; got %#v", want, facts.BranchRefinements(point))
		}
		falseValue, ok := refinement.FalseValue()
		if !ok || !falseValue.HasPresence(presence.Absent()) {
			t.Fatalf("false-edge refinement for %s = %#v/%v, want absent", want, falseValue, ok)
		}
	}
}

func TestLowerNegatedConjunctionExpressionConditionKeepsLeafPolarity(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(x: string?, y: string?)
	local ok = not (x == nil and y == nil)
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	xPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "x")
	yPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "y")
	local := mustLocalStmt(t, fn.Stmts, 0)
	source := mustLocalSource(t, facts, requireStmtPoints(t, built, local, 1)[0])
	condition, ok := facts.ExpressionCondition(source.ExprRef)
	if !ok {
		t.Fatalf("missing expression condition for local source ref %d", source.ExprRef)
	}

	falseFacts := condition.FactsForValue(false)
	falseRefinements := falseFacts.Refinements()
	for _, want := range []path.Path{xPath, yPath} {
		found := false
		for _, refinement := range falseRefinements {
			if !refinement.TargetPath().Equal(want) {
				continue
			}
			if !refinement.Value().HasPresence(presence.Absent()) {
				t.Fatalf("false-value expression refinement for %s = %#v, want absent", want, refinement.Value())
			}
			found = true
			break
		}
		if !found {
			t.Fatalf("missing false-value expression refinement for %s; got %#v", want, falseRefinements)
		}
	}
}

func TestLowerBooleanLocalAliasPublishesConditionRefinements(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(target: { transform: string? })
	local has_transform = target.transform ~= nil
	if has_transform then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	targetPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "target").Field("transform")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), targetPath)
	if !ok {
		t.Fatalf("missing branch refinement for %s; got %#v", targetPath, facts.BranchRefinements(point))
	}
	trueValue, ok := refinement.TrueValue()
	if !ok || !trueValue.HasPresence(presence.Present()) {
		t.Fatalf("true-edge alias refinement = %#v/%v, want present", trueValue, ok)
	}
}

func TestLowerBooleanLocalAliasPublishesConditionPathRelations(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
	local same = a == b
	if same then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	aPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "a")
	bPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "b")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	assertBranchPathEqualityRelation(t, facts, point, aPath, bPath, true, false)
}

func TestLowerBooleanLocalAliasPublishesConditionPathEvidence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
	local same = a == b
	if same then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	aPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "a")
	bPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "b")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	assertBranchPathEqualityEvidence(t, facts, point, aPath, bPath, true, false)
}

func TestLowerNegatedBooleanLocalAliasInvertsConditionPathRelations(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
	local same = a == b
	if not same then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	aPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "a")
	bPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "b")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	assertBranchPathEqualityRelation(t, facts, point, aPath, bPath, false, true)
}

func TestLowerNegatedBooleanLocalAliasInvertsConditionPathEvidence(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(a: string?, b: string?)
	local same = a == b
	if not same then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	aPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "a")
	bPath := path.NewPath(bindings.ParamSlots(fn)[1].Symbol, "b")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	assertBranchPathEqualityEvidence(t, facts, point, aPath, bPath, false, true)
}

func TestLowerNegatedBooleanLocalAliasInvertsConditionRefinements(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(target: { transform: string? })
	local has_transform = target.transform ~= nil
	if not has_transform then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	targetPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "target").Field("transform")
	ifStmt := fn.Stmts[1].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]

	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), targetPath)
	if !ok {
		t.Fatalf("missing branch refinement for %s; got %#v", targetPath, facts.BranchRefinements(point))
	}
	falseValue, ok := refinement.FalseValue()
	if !ok || !falseValue.HasPresence(presence.Present()) {
		t.Fatalf("false-edge alias refinement = %#v/%v, want present", falseValue, ok)
	}
}

func assertBranchPathEqualityRelation(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if wantRight.Less(wantLeft) {
		wantLeft, wantRight = wantRight, wantLeft
	}
	relations := facts.BranchPathRelations(point)
	if len(relations) != 2 {
		t.Fatalf("branch path relations at point %d = %d, want 2: %#v", point, len(relations), relations)
	}
	var equality, inequality factflow.BranchPathRelation
	for _, relation := range relations {
		switch relation.Kind() {
		case factflow.BranchPathRelationEqual:
			equality = relation
		case factflow.BranchPathRelationNotEqual:
			inequality = relation
		default:
			t.Fatalf("branch path relation kind = %v, want equality or inequality", relation.Kind())
		}
	}
	if equality.Kind() != factflow.BranchPathRelationEqual {
		t.Fatal("missing equality branch path relation")
	}
	if inequality.Kind() != factflow.BranchPathRelationNotEqual {
		t.Fatal("missing inequality branch path relation")
	}
	for _, relation := range []factflow.BranchPathRelation{equality, inequality} {
		if !relation.LeftPath().Equal(wantLeft) {
			t.Fatalf("branch path relation left = %#v, want %#v", relation.LeftPath(), wantLeft)
		}
		if !relation.RightPath().Equal(wantRight) {
			t.Fatalf("branch path relation right = %#v, want %#v", relation.RightPath(), wantRight)
		}
	}
	if equality.ActiveOnEdge(true) != wantTrue || equality.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("equality relation active true/false = %v/%v, want %v/%v", equality.ActiveOnEdge(true), equality.ActiveOnEdge(false), wantTrue, wantFalse)
	}
	if inequality.ActiveOnEdge(true) != !wantTrue || inequality.ActiveOnEdge(false) != !wantFalse {
		t.Fatalf("inequality relation active true/false = %v/%v, want %v/%v", inequality.ActiveOnEdge(true), inequality.ActiveOnEdge(false), !wantTrue, !wantFalse)
	}
}

func assertBranchPathEqualityEvidence(
	t *testing.T,
	facts factflow.Facts,
	point cfg.Point,
	wantLeft path.Path,
	wantRight path.Path,
	wantTrue bool,
	wantFalse bool,
) {
	t.Helper()
	if wantRight.Less(wantLeft) {
		wantLeft, wantRight = wantRight, wantLeft
	}
	proofs := facts.BranchPathEvidence(point)
	var equality, inequality factflow.BranchPathEvidence
	for _, proof := range proofs {
		switch proof.Kind() {
		case factflow.BranchPathEvidenceEqual:
			if branchPathEvidenceMatchesPaths(proof, wantLeft, wantRight) {
				equality = proof
			}
		case factflow.BranchPathEvidenceNotEqual:
			if branchPathEvidenceMatchesPaths(proof, wantLeft, wantRight) {
				inequality = proof
			}
		}
	}
	if equality.Kind() != factflow.BranchPathEvidenceEqual {
		t.Fatalf("missing equality branch path evidence for %s == %s; got %#v", wantLeft, wantRight, proofs)
	}
	if inequality.Kind() != factflow.BranchPathEvidenceNotEqual {
		t.Fatalf("missing inequality branch path evidence for %s ~= %s; got %#v", wantLeft, wantRight, proofs)
	}
	if equality.ActiveOnEdge(true) != wantTrue || equality.ActiveOnEdge(false) != wantFalse {
		t.Fatalf("equality evidence active true/false = %v/%v, want %v/%v", equality.ActiveOnEdge(true), equality.ActiveOnEdge(false), wantTrue, wantFalse)
	}
	if inequality.ActiveOnEdge(true) != !wantTrue || inequality.ActiveOnEdge(false) != !wantFalse {
		t.Fatalf("inequality evidence active true/false = %v/%v, want %v/%v", inequality.ActiveOnEdge(true), inequality.ActiveOnEdge(false), !wantTrue, !wantFalse)
	}
}

func branchPathEvidenceMatchesPaths(proof factflow.BranchPathEvidence, wantLeft path.Path, wantRight path.Path) bool {
	if !proof.Path().Equal(wantLeft) {
		return false
	}
	other, ok := proof.OtherPath()
	return ok && other.Equal(wantRight)
}

func TestLowerTypedOptionalMemberBranchPublishesStaticRuntimeKind(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(page: { data_func: string? }?)
	if not page or not page.data_func or page.data_func == "" then
	end
end
`)

	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	page := bindings.ParamSlots(fn)[0].Symbol
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	point := requireStmtPoints(t, built, ifStmt, 1)[0]
	target := path.NewPath(page, "page").Field("data_func")
	refinement, ok := branchRefinementAt(facts.BranchRefinements(point), target)
	if !ok {
		t.Fatalf("missing branch refinement for %s", target)
	}
	if _, ok := refinement.TrueValue(); ok {
		t.Fatalf("true edge has refinement for %s, want false edge only", target)
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge refinement for %s", target)
	}
	assertValueRefinement(t, "false edge", falseValue, valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.String),
		hasRuntimeKind: true,
	})
}

func TestLowerMemberPathBranchRefinementOrdersRootBeforeChild(t *testing.T) {
	template := typetable.NewRecord().
		Field("kind", typ.LiteralString("template")).
		Field("data_func", typeexpr.Optional(typ.String)).
		Build()
	component := typetable.NewRecord().
		Field("kind", typ.LiteralString("component")).
		Field("url", typ.String).
		Build()
	pageType := typeexpr.Union(template, component)
	page := symbol.ID(701)
	rootPath := path.NewPath(page, "page")
	childPath := rootPath.Field("data_func")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{page: pageType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind: branchcond.CheckNotNil,
		Path: childPath,
	})
	rootIndex := branchRefinementIndex(refinements, rootPath)
	if rootIndex < 0 {
		t.Fatalf("missing root refinement for %s", rootPath)
	}
	childIndex := branchRefinementIndex(refinements, childPath)
	if childIndex < 0 {
		t.Fatalf("missing child refinement for %s", childPath)
	}
	if rootIndex >= childIndex {
		t.Fatalf("root refinement index = %d, child index = %d; want root first", rootIndex, childIndex)
	}
}

func TestLowerTableIsFrozenDirectConditionPublishesFrozenTableProof(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = {}
if table.isfrozen(t) then
end
`, "table")
	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, reg)
	ifStmt := mustIfStmt(t, stmts, 1)
	branchPoint := requireStmtPoints(t, built, ifStmt, 2)[1]
	target := path.NewPath(mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0), "t")
	branchFact, _ := result.BranchCondition(branchPoint)

	proofs := facts.BranchPathEvidence(branchPoint)
	if len(proofs) != 1 {
		t.Fatalf("branch path evidence = %#v, want 1 frozen-table proof; condition=%T %#v", proofs, branchFact.Condition, branchFact.Condition)
	}
	proof := proofs[0]
	if proof.Kind() != factflow.BranchPathEvidenceFrozenTable {
		t.Fatalf("branch path evidence kind = %v, want frozen-table", proof.Kind())
	}
	if !proof.Path().Equal(target) {
		t.Fatalf("branch path evidence path = %s, want %s", proof.Path(), target)
	}
	if !proof.ActiveOnEdge(true) || proof.ActiveOnEdge(false) {
		t.Fatalf("branch path evidence active true/false = %v/%v, want true/false", proof.ActiveOnEdge(true), proof.ActiveOnEdge(false))
	}
}

func TestLowerTableIsFrozenNegatedConditionPublishesFrozenTableProofOnFalseEdge(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = {}
if not table.isfrozen(t) then
end
`, "table")
	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, reg)
	ifStmt := mustIfStmt(t, stmts, 1)
	branchPoint := requireStmtPoints(t, built, ifStmt, 2)[1]
	target := path.NewPath(mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0), "t")
	branchFact, _ := result.BranchCondition(branchPoint)

	proofs := facts.BranchPathEvidence(branchPoint)
	if len(proofs) != 1 {
		t.Fatalf("branch path evidence = %#v, want 1 frozen-table proof; condition=%T %#v", proofs, branchFact.Condition, branchFact.Condition)
	}
	proof := proofs[0]
	if proof.Kind() != factflow.BranchPathEvidenceFrozenTable {
		t.Fatalf("branch path evidence kind = %v, want frozen-table", proof.Kind())
	}
	if !proof.Path().Equal(target) {
		t.Fatalf("branch path evidence path = %s, want %s", proof.Path(), target)
	}
	if proof.ActiveOnEdge(true) || !proof.ActiveOnEdge(false) {
		t.Fatalf("branch path evidence active true/false = %v/%v, want false/true", proof.ActiveOnEdge(true), proof.ActiveOnEdge(false))
	}
}

func TestLowerTableIsFrozenConjunctionPublishesFrozenTableProofWithOtherGuards(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local t = {}
local ok: boolean = true
if table.isfrozen(t) and ok then
end
`, "table")
	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, reg)
	ifStmt := mustIfStmt(t, stmts, 2)
	branchPoint := requireStmtPoints(t, built, ifStmt, 2)[1]
	target := path.NewPath(mustLocalAt(t, bindings, mustLocalStmt(t, stmts, 0), 0), "t")

	var foundFrozen bool
	var foundOtherGuard bool
	for _, proof := range facts.BranchPathEvidence(branchPoint) {
		if proof.Kind() == factflow.BranchPathEvidenceFrozenTable && proof.Path().Equal(target) {
			if !proof.ActiveOnEdge(true) || proof.ActiveOnEdge(false) {
				t.Fatalf("frozen-table proof active true/false = %v/%v, want true/false", proof.ActiveOnEdge(true), proof.ActiveOnEdge(false))
			}
			foundFrozen = true
			continue
		}
		if proof.Kind() == factflow.BranchPathEvidencePresence && proof.ActiveOnEdge(true) {
			foundOtherGuard = true
		}
	}
	if !foundFrozen {
		t.Fatalf("missing frozen-table proof in conjunction: %#v", facts.BranchPathEvidence(branchPoint))
	}
	if !foundOtherGuard {
		t.Fatalf("missing ordinary guard evidence in conjunction: %#v", facts.BranchPathEvidence(branchPoint))
	}
}

func TestLowerTableIsFrozenIgnoresShadowedLocalTable(t *testing.T) {
	reg := standard.Registry()
	stmts, bindings, built, result := parseSemanticChunk(t, `
local table = { isfrozen = function(value) return true end }
local t = {}
if table.isfrozen(t) then
end
`, "table")
	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, reg)
	ifStmt := mustIfStmt(t, stmts, 2)
	branchPoint := requireStmtPoints(t, built, ifStmt, 2)[1]
	branchFact, _ := result.BranchCondition(branchPoint)
	if got := facts.BranchPathEvidence(branchPoint); len(got) != 0 {
		t.Fatalf("branch path evidence = %#v, want none for shadowed table; condition=%T %#v", got, branchFact.Condition, branchFact.Condition)
	}
}

func TestLowerCompoundBranchOrdersAllRootsBeforeDescendants(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(page: { data_func: string?, url: string? } | { other: string })
	if page.data_func and page.url then
	end
end
`)
	facts := lowerFunctionFactsWithWIR(t, "branch", result, built, bindings, standard.Registry())
	page := bindings.ParamSlots(fn)[0].Symbol
	rootPath := path.NewPath(page, "page")
	dataPath := rootPath.Field("data_func")
	urlPath := rootPath.Field("url")
	ifStmt := fn.Stmts[0].(*ast.IfStmt)
	refinements := facts.BranchRefinements(requireStmtPoints(t, built, ifStmt, 1)[0])

	firstDescendant := len(refinements)
	for i, refinement := range refinements {
		if len(refinement.TargetPath().Segments) != 0 {
			firstDescendant = i
			break
		}
	}
	if firstDescendant == len(refinements) {
		t.Fatalf("compound branch produced no descendant refinements: %#v", refinements)
	}
	for i := firstDescendant; i < len(refinements); i++ {
		if len(refinements[i].TargetPath().Segments) == 0 {
			t.Fatalf("root refinement at index %d after descendant index %d", i, firstDescendant)
		}
	}
	if branchRefinementIndex(refinements, rootPath) < 0 {
		t.Fatalf("missing root refinement for compound branch")
	}
	if branchRefinementIndex(refinements, dataPath) < 0 {
		t.Fatalf("missing data_func descendant refinement")
	}
	if branchRefinementIndex(refinements, urlPath) < 0 {
		t.Fatalf("missing url descendant refinement")
	}
}

func TestLowerLiteralDiscriminantBranchRefinesRootOnBothEdges(t *testing.T) {
	left := typetable.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	rootType := typeexpr.Union(left, right)
	root := symbol.ID(801)
	rootPath := path.NewPath(root, "r")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: rootType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind:          branchcond.CheckLiteralEqual,
		Path:          rootPath.Field("tag"),
		Literal:       typ.LiteralString("a"),
		LiteralString: "a",
	})
	refinement, ok := branchRefinementAt(refinements, rootPath)
	if !ok {
		t.Fatalf("missing root refinement for literal discriminant")
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatalf("missing true-edge root refinement")
	}
	falseValue, ok := refinement.FalseValue()
	if !ok {
		t.Fatalf("missing false-edge root refinement")
	}
	assertVariantOriginRefinementType(t, "true edge", trueValue, left)
	assertVariantOriginRefinementType(t, "false edge", falseValue, right)
}

func TestLowerLiteralDiscriminantBranchAlsoPublishesDescendantProof(t *testing.T) {
	left := typetable.NewRecord().
		Field("tag", typ.LiteralString("a")).
		Field("value", typ.String).
		Build()
	right := typetable.NewRecord().
		Field("tag", typ.LiteralString("b")).
		Field("value", typ.Number).
		Build()
	rootType := typeexpr.Union(left, right)
	root := symbol.ID(802)
	rootPath := path.NewPath(root, "r")
	tagPath := rootPath.Field("tag")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: rootType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind:          branchcond.CheckLiteralEqual,
		Path:          tagPath,
		Literal:       typ.LiteralString("a"),
		LiteralString: "a",
	})
	if _, ok := branchRefinementAt(refinements, rootPath); !ok {
		t.Fatalf("missing root variant refinement: %#v", refinements)
	}
	descendant, ok := branchRefinementAt(refinements, tagPath)
	if !ok {
		t.Fatalf("missing descendant literal refinement: %#v", refinements)
	}
	trueValue, ok := descendant.TrueValue()
	if !ok {
		t.Fatal("missing true-edge descendant literal refinement")
	}
	constraint, ok := trueValue.Constraint()
	if !ok {
		t.Fatal("true-edge descendant literal refinement missing constraint")
	}
	gotType, ok := typevalue.TypeOf(standard.Registry(), constraint)
	if !ok || !typ.TypeEquals(gotType, typ.LiteralString("a")) {
		t.Fatalf("descendant literal type = %v/%v, want \"a\"", gotType, ok)
	}
}

func TestLowerOpenScalarDescendantLiteralBranchKeepsPathProof(t *testing.T) {
	root := symbol.ID(804)
	rootPath := path.NewPath(root, "cell")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: typetable.NewRecord().Field("kind", typ.String).Build()},
		typeValues:  typevalue.NewCache(),
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind:          branchcond.CheckLiteralEqual,
		Path:          rootPath.Field("kind"),
		Literal:       typ.LiteralString("boolean"),
		LiteralString: "boolean",
	})
	refinement, ok := branchRefinementAt(refinements, rootPath.Field("kind"))
	if !ok {
		t.Fatalf("missing descendant literal refinement; got %#v", refinements)
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatal("missing true-edge literal refinement")
	}
	constraint, ok := trueValue.Constraint()
	if !ok {
		t.Fatal("true-edge constraint missing")
	}
	gotType, ok := typevalue.TypeOf(standard.Registry(), constraint)
	if !ok || !typ.TypeEquals(gotType, typ.LiteralString("boolean")) {
		t.Fatalf("true-edge type = %v/%v, want literal boolean type name", gotType, ok)
	}
}

func TestLowerOpenRootScalarLiteralBranchKeepsNegatedEdgeProof(t *testing.T) {
	root := symbol.ID(805)
	rootPath := path.NewPath(root, "kind")
	l := lowerer{
		registry:   standard.Registry(),
		typeValues: typevalue.NewCache(),
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind:          branchcond.CheckLiteralEqual,
		Path:          rootPath,
		Literal:       typ.LiteralString("auto"),
		LiteralString: "auto",
	})
	refinement, ok := branchRefinementAt(refinements, rootPath)
	if !ok {
		t.Fatalf("missing root literal refinement; got %#v", refinements)
	}
	trueValue, ok := refinement.TrueValue()
	if !ok {
		t.Fatal("missing true-edge root literal refinement")
	}
	falseValue, ok := refinement.FalseValue()
	if !ok || !falseValue.NegatedLiteral() {
		t.Fatalf("false-edge root literal refinement = %#v/%v, want negated literal", falseValue, ok)
	}
	for label, value := range map[string]factflow.ValueRefinement{
		"true edge":  trueValue,
		"false edge": falseValue,
	} {
		constraint, ok := value.Constraint()
		if !ok {
			t.Fatalf("%s constraint missing", label)
		}
		gotType, ok := typevalue.TypeOf(standard.Registry(), constraint)
		if !ok || !typ.TypeEquals(gotType, typ.LiteralString("auto")) {
			t.Fatalf("%s type = %v/%v, want literal auto", label, gotType, ok)
		}
	}
}

func TestLowerTruthyInstantiatedResultBranchRefinesRootOnBothEdges(t *testing.T) {
	resultType, valueCase, errorCase := instantiatedResultTypeParamFixture()
	root := symbol.ID(802)
	rootPath := path.NewPath(root, "result")
	l := lowerer{
		registry:    standard.Registry(),
		symbolTypes: map[symbol.ID]typ.Type{root: resultType},
	}

	refinements := l.branchRefinementsForCheck(branchcond.Check{
		Kind: branchcond.CheckTruthy,
		Path: rootPath.Field("ok"),
	})
	rootFamily, _, ok := variant.OriginOfType(resultType)
	if !ok {
		t.Fatal("missing root origin for Result<T>")
	}

	trueValue := requireBranchRefinementValueAt(t, refinements, rootPath, true)
	falseValue := requireBranchRefinementValueAt(t, refinements, rootPath, false)
	assertVariantOriginRefinementType(t, "true edge", trueValue, valueCase)
	assertVariantOriginRefinementType(t, "false edge", falseValue, errorCase)
	assertVariantOriginRefinementFamily(t, "true edge", trueValue, rootFamily)
	assertVariantOriginRefinementFamily(t, "false edge", falseValue, rootFamily)
}

func assertVariantOriginRefinementType(t *testing.T, label string, refinement factflow.ValueRefinement, want typ.Type) {
	t.Helper()
	constraint, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	origin := product.Get(standard.Registry(), constraint, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("%s variant origin = %v, want concrete", label, origin)
	}
	got, ok := variant.TypeFromOrigin(origin.Family(), origin.Cases())
	if !ok {
		t.Fatalf("%s origin type unavailable", label)
	}
	if !typ.SameNodeOrAcyclicEqual(got, want) {
		t.Fatalf("%s origin type = %v, want %v", label, got, want)
	}
}

func assertVariantOriginRefinementFamily(t *testing.T, label string, refinement factflow.ValueRefinement, want uint64) {
	t.Helper()
	constraint, ok := refinement.Constraint()
	if !ok {
		t.Fatalf("%s constraint missing", label)
	}
	origin := product.Get(standard.Registry(), constraint, variantorigin.Key)
	if origin.IsBottom() || origin.IsTop() {
		t.Fatalf("%s variant origin = %v, want family %d", label, origin, want)
	}
	if origin.Family() != want {
		t.Fatalf("%s origin family = %d, want %d", label, origin.Family(), want)
	}
}

func TestLowerPathEqualityBranchRelation(t *testing.T) {
	decl := localAssign([]string{"a", "b"}, number("1"), number("2"))
	aRead := ident("a")
	bRead := ident("b")
	eqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{Operator: "==", Lhs: aRead, Rhs: bRead}}
	stmts := []ast.Stmt{decl, eqStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, eqStmt, 1)[0]
	assertLoweredBranchPathEquality(
		t,
		facts,
		point,
		path.NewPath(mustIdentSymbol(t, bindings, aRead), "a"),
		path.NewPath(mustIdentSymbol(t, bindings, bRead), "b"),
		true,
		false,
	)
}

func TestLowerSequentialPathEqualityBranchRelation(t *testing.T) {
	stmts := parseChunk(t, `
local result, events_ch, timeout = {}, {}, {}
if result.channel == timeout then
	return
end
if result.channel == events_ch then
	local event = result.value
end
`)
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	secondIf := stmts[2].(*ast.IfStmt)
	point := requireStmtPoints(t, built, secondIf, 1)[0]
	relations := facts.BranchPathRelations(point)
	if len(relations) == 0 {
		t.Fatalf("second branch path relations at point %d = 0", point)
	}
}

func branchRefinementIndex(refinements []factflow.BranchRefinement, wantPath path.Path) int {
	for i, refinement := range refinements {
		if refinement.TargetPath().Equal(wantPath) {
			return i
		}
	}
	return -1
}

func requireBranchRefinementValueAt(t *testing.T, refinements []factflow.BranchRefinement, wantPath path.Path, cond bool) factflow.ValueRefinement {
	t.Helper()
	for _, refinement := range refinements {
		if !refinement.TargetPath().Equal(wantPath) {
			continue
		}
		if value, ok := refinement.ValueForEdge(cond); ok {
			return value
		}
	}
	t.Fatalf("missing %t-edge refinement for %s in %#v", cond, wantPath, refinements)
	return factflow.ValueRefinement{}
}

func assertBranchRefinementConstraint(t *testing.T, reg *axis.Registry, refinement factflow.BranchRefinement, cond bool, want product.Value) {
	t.Helper()
	value, ok := refinement.ValueForEdge(cond)
	if !ok {
		t.Fatalf("missing %t-edge refinement value in %#v", cond, refinement)
	}
	constraint, ok := value.Constraint()
	if !ok {
		t.Fatalf("%t-edge refinement has no constraint: %#v", cond, value)
	}
	if !product.Equal(reg, constraint, want) {
		t.Fatalf("%t-edge constraint = %#v, want %#v", cond, constraint, want)
	}
}

func assertRootRefinementsBeforeDescendants(t *testing.T, refinements []factflow.BranchRefinement) {
	t.Helper()
	seenDescendant := false
	for i, refinement := range refinements {
		if len(refinement.TargetPath().Segments) == 0 {
			if seenDescendant {
				t.Fatalf("root refinement at index %d appears after descendant in %#v", i, refinements)
			}
			continue
		}
		seenDescendant = true
	}
}

func instantiatedResultTypeParamFixture() (typ.Type, typ.Type, typ.Type) {
	tp := typ.NewTypeParam("T", nil)
	valueCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(true)).
		Field("value", tp).
		Build()
	errorCase := typetable.NewRecord().
		Field("ok", typ.LiteralBool(false)).
		Field("error", typ.String).
		Build()
	result := typ.NewGeneric("Result", []*typ.TypeParam{tp}, typeexpr.Union(valueCase, errorCase))
	return typ.Instantiate(result, tp), valueCase, errorCase
}

func TestLowerPathInequalityBranchRelation(t *testing.T) {
	decl := localAssign([]string{"t", "u"}, &ast.TableExpr{}, &ast.TableExpr{})
	tRead := ident("t")
	uRead := ident("u")
	neqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      dot(tRead, "left"),
		Rhs:      dot(uRead, "right"),
	}}
	stmts := []ast.Stmt{decl, neqStmt}
	bindings := bind.BindChunk(stmts, bind.Options{})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, neqStmt, 1)[0]
	assertLoweredBranchPathEquality(
		t,
		facts,
		point,
		path.NewPath(mustIdentSymbol(t, bindings, tRead), "t").Field("left"),
		path.NewPath(mustIdentSymbol(t, bindings, uRead), "u").Field("right"),
		false,
		true,
	)
}

func TestLowerTypeGuardTableEqualityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("table"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardFunctionInequalityBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(xRead),
		Rhs:      stringLit("function"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Function),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Function),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerTypeGuardNilBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	eqRead := ident("x")
	eqStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(eqRead),
		Rhs:      stringLit("nil"),
	}}
	notRead := ident("x")
	notStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "~=",
		Lhs:      typeCall(notRead),
		Rhs:      stringLit("nil"),
	}}
	stmts := []ast.Stmt{decl, eqStmt, notStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	xPath := path.NewPath(mustIdentSymbol(t, bindings, eqRead), "x")
	nilValue := valueRefinementExpectation{
		presence:       presence.Absent(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Singleton(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	notNilValue := valueRefinementExpectation{
		presence:       presence.Present(),
		hasPresence:    true,
		runtimeKind:    runtimekind.Top().Without(runtimekind.Nil),
		hasRuntimeKind: true,
	}
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, eqStmt, 1)[0], xPath, nilValue, notNilValue)
	assertLoweredBranchValueRefinement(t, facts, requireStmtPoints(t, built, notStmt, 1)[0], xPath, notNilValue, nilValue)
}

func TestLowerTypeGuardRuntimeTypeNames(t *testing.T) {
	l := lowerer{registry: standard.Registry()}
	target := path.NewPath(symbol.ID(1), "x")
	tests := []struct {
		typeName string
		tag      runtimekind.Tag
	}{
		{"nil", runtimekind.Nil},
		{"boolean", runtimekind.Boolean},
		{"number", runtimekind.Number},
		{"string", runtimekind.String},
		{"table", runtimekind.Table},
		{"function", runtimekind.Function},
		{"thread", runtimekind.Thread},
		{"userdata", runtimekind.Userdata},
	}

	for _, tt := range tests {
		refinement, ok := l.typeBranchRefinement(target, branchcond.CheckTypeEqual, tt.typeName)
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) returned false", tt.typeName)
		}
		trueValue, ok := refinement.TrueValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing true-edge refinement", tt.typeName)
		}
		falseValue, ok := refinement.FalseValue()
		if !ok {
			t.Fatalf("typeBranchRefinement(%q) missing false-edge refinement", tt.typeName)
		}

		truePresence := presence.Present()
		falsePresence := presence.Top()
		falseHasPresence := false
		if tt.tag == runtimekind.Nil {
			truePresence = presence.Absent()
			falsePresence = presence.Present()
			falseHasPresence = true
		}
		assertValueRefinement(t, tt.typeName+" true edge", trueValue, valueRefinementExpectation{
			presence:       truePresence,
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(tt.tag),
			hasRuntimeKind: true,
		})
		trueConstraint, trueConstraintOK := trueValue.Constraint()
		if !trueConstraintOK {
			t.Fatalf("%s true edge missing constraint", tt.typeName)
		}
		if got := product.Get(l.registry, trueConstraint, assertion.Key); !got.Has(assertion.RuntimeClaim) {
			t.Fatalf("%s true edge assertion = %s, want runtime claim", tt.typeName, got)
		}
		assertValueRefinement(t, tt.typeName+" false edge", falseValue, valueRefinementExpectation{
			presence:       falsePresence,
			hasPresence:    falseHasPresence,
			runtimeKind:    runtimekind.Top().Without(tt.tag),
			hasRuntimeKind: true,
		})
		falseConstraint, falseConstraintOK := falseValue.Constraint()
		if !falseConstraintOK {
			t.Fatalf("%s false edge missing constraint", tt.typeName)
		}
		if got := product.Get(l.registry, falseConstraint, assertion.Key); !got.Has(assertion.RuntimeClaim) {
			t.Fatalf("%s false edge assertion = %s, want runtime claim", tt.typeName, got)
		}
	}
}

func TestLowerTypeGuardReversedOperandsBranchRefinement(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	xRead := ident("x")
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      stringLit("table"),
		Rhs:      typeCall(xRead),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	xPath := path.NewPath(mustIdentSymbol(t, bindings, xRead), "x")
	assertLoweredBranchValueRefinement(t, facts, point, xPath,
		valueRefinementExpectation{
			presence:       presence.Present(),
			hasPresence:    true,
			runtimeKind:    runtimekind.Singleton(runtimekind.Table),
			hasRuntimeKind: true,
		},
		valueRefinementExpectation{
			runtimeKind:    runtimekind.Top().Without(runtimekind.Table),
			hasRuntimeKind: true,
		},
	)
}

func TestLowerSkipsUnknownTypeGuardBranchRefinements(t *testing.T) {
	decl := localAssign([]string{"x"}, number("0"))
	typeStmt := &ast.IfStmt{Condition: &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      typeCall(ident("x")),
		Rhs:      stringLit("mystery"),
	}}
	stmts := []ast.Stmt{decl, typeStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerChunkFactsWithWIR(t, "branch", stmts, result, built, bindings, standard.Registry())
	point := requireStmtPoints(t, built, typeStmt, 1)[0]
	if len(facts.BranchRefinements(point)) != 0 {
		t.Fatalf("unknown type guard branch point %d lowered as branch refinement", point)
	}
}
