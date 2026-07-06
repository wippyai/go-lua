package transferfacts

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/callproducer"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/ir/wir"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/cfgbuild"
	"github.com/wippyai/go-lua/analysis/lua/pathexpr"
	"github.com/wippyai/go-lua/analysis/lua/semantics"
	"github.com/wippyai/go-lua/analysis/lua/sourceprovenance"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/lua/wirlower"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/source"
)

func TestLowerCallSitesPreserveAllSemanticContextsAndProducerStaysNarrow(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	local := localAssign([]string{"a"}, makeCall)
	printCall := &ast.FuncCallExpr{Func: ident("print")}
	printStmt := &ast.FuncCallStmt{Expr: printCall}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: readyCall}
	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	genericFor := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterCall}}
	tailCall := &ast.FuncCallExpr{Func: ident("tail")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{tailCall}}
	stmts := []ast.Stmt{local, printStmt, ifStmt, genericFor, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "iter", "tail"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})

	localPoints := requireStmtPoints(t, built, local, 2)
	localSite, ok := facts.CallSite(localPoints[0])
	if !ok {
		t.Fatalf("missing assignment-source call site")
	}
	if localSite.Context() != factflow.CallSiteContextAssignmentSource || localSite.ExprIndex() != 0 || localSite.CalleeSymbol() == 0 {
		t.Fatalf("assignment-source call site = context %v expr index %d callee %d", localSite.Context(), localSite.ExprIndex(), localSite.CalleeSymbol())
	}
	if expr, ok := localSite.Expr(); !ok || expr == 0 {
		t.Fatalf("assignment-source call-site expr = %d/%v", expr, ok)
	}
	localTargets := localSite.ResultTargets()
	if len(localTargets) != 1 || localTargets[0].Kind() != factflow.CallResultTargetLocalAssignment || localTargets[0].Index() != 0 || localTargets[0].ResultIndex() != 0 {
		t.Fatalf("assignment-source call-site targets = %#v", localTargets)
	}
	if _, ok := callproducer.FromFacts(facts, localPoints[0]); !ok {
		t.Fatalf("assignment-source call point %d missing producer", localPoints[0])
	}

	statementPoint := requireStmtPoints(t, built, printStmt, 1)[0]
	statementSite, ok := facts.CallSite(statementPoint)
	if !ok {
		t.Fatalf("missing statement call site")
	}
	if statementSite.Context() != factflow.CallSiteContextStatement || !statementSite.Final() || !statementSite.Adjusted() || statementSite.Expanded() {
		t.Fatalf("statement call site flags/context = %v final=%v adjusted=%v expanded=%v", statementSite.Context(), statementSite.Final(), statementSite.Adjusted(), statementSite.Expanded())
	}
	if len(statementSite.ResultTargets()) != 0 {
		t.Fatalf("statement call-site targets = %#v, want none", statementSite.ResultTargets())
	}
	if _, ok := callproducer.FromFacts(facts, statementPoint); ok {
		t.Fatalf("statement call point %d lowered as call producer", statementPoint)
	}

	branchPoints := requireStmtPoints(t, built, ifStmt, 2)
	conditionSite, ok := facts.CallSite(branchPoints[0])
	if !ok {
		t.Fatalf("missing condition call site")
	}
	if conditionSite.Context() != factflow.CallSiteContextCondition || conditionSite.ExprIndex() != 0 || !conditionSite.Final() || !conditionSite.Adjusted() {
		t.Fatalf("condition call site = context %v expr index %d final=%v adjusted=%v", conditionSite.Context(), conditionSite.ExprIndex(), conditionSite.Final(), conditionSite.Adjusted())
	}
	if conditionSite.ConditionNegated() {
		t.Fatalf("condition call site unexpectedly negated")
	}
	if _, ok := callproducer.FromFacts(facts, branchPoints[0]); ok {
		t.Fatalf("condition call point %d lowered as call producer", branchPoints[0])
	}

	genericPoints := requireStmtPoints(t, built, genericFor, 3)
	iteratorSite, ok := facts.CallSite(genericPoints[0])
	if !ok {
		t.Fatalf("missing iterator call site")
	}
	if iteratorSite.Context() != factflow.CallSiteContextIteratorSource || iteratorSite.ExprIndex() != 0 {
		t.Fatalf("iterator call site = context %v expr index %d", iteratorSite.Context(), iteratorSite.ExprIndex())
	}
	if _, ok := callproducer.FromFacts(facts, genericPoints[0]); ok {
		t.Fatalf("iterator call point %d lowered as call producer", genericPoints[0])
	}

	returnPoints := requireStmtPoints(t, built, ret, 2)
	returnSite, ok := facts.CallSite(returnPoints[0])
	if !ok {
		t.Fatalf("missing return-source call site")
	}
	if returnSite.Context() != factflow.CallSiteContextReturnSource || returnSite.ExprIndex() != 0 || !returnSite.OpenTail() {
		t.Fatalf("return-source call site = context %v expr index %d open tail=%v", returnSite.Context(), returnSite.ExprIndex(), returnSite.OpenTail())
	}
	returnTargets := returnSite.ResultTargets()
	if len(returnTargets) != 1 || returnTargets[0].Kind() != factflow.CallResultTargetReturn || returnTargets[0].Index() != 0 || returnTargets[0].ResultIndex() != 0 {
		t.Fatalf("return-source call-site targets = %#v", returnTargets)
	}
	if _, ok := callproducer.FromFacts(facts, returnPoints[0]); !ok {
		t.Fatalf("return-source call point %d missing producer", returnPoints[0])
	}
}

func TestLowerCallSiteContextFlagsFromWIRLowerer(t *testing.T) {
	makeCall := &ast.FuncCallExpr{Func: ident("make")}
	local := localAssign([]string{"a"}, makeCall)
	printCall := &ast.FuncCallExpr{Func: ident("print")}
	printStmt := &ast.FuncCallStmt{Expr: printCall}
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: readyCall}
	iterCall := &ast.FuncCallExpr{Func: ident("iter")}
	genericFor := &ast.GenericForStmt{Names: []string{"item"}, Exprs: []ast.Expr{iterCall}}
	tailCall := &ast.FuncCallExpr{Func: ident("tail")}
	ret := &ast.ReturnStmt{Exprs: []ast.Expr{tailCall}}
	stmts := []ast.Stmt{local, printStmt, ifStmt, genericFor, ret}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"make", "print", "ready", "iter", "tail"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}
	body := wirlower.Lower("contexts", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	assertSite := func(label string, point cfg.Point, context factflow.CallSiteContext, exprIndex int, final, expanded, adjusted, openTail bool) {
		t.Helper()
		site, ok := facts.CallSite(point)
		if !ok {
			t.Fatalf("%s: missing call site at point %d", label, point)
		}
		if site.Context() != context || site.ExprIndex() != exprIndex ||
			site.Final() != final || site.Expanded() != expanded ||
			site.Adjusted() != adjusted || site.OpenTail() != openTail {
			t.Fatalf("%s: call site = context:%v expr:%d final:%v expanded:%v adjusted:%v open:%v",
				label, site.Context(), site.ExprIndex(), site.Final(), site.Expanded(), site.Adjusted(), site.OpenTail())
		}
	}

	assertSite("assignment", requireStmtPoints(t, built, local, 2)[0], factflow.CallSiteContextAssignmentSource, 0, true, true, false, false)
	assertSite("statement", requireStmtPoints(t, built, printStmt, 1)[0], factflow.CallSiteContextStatement, 0, true, false, true, false)
	assertSite("condition", requireStmtPoints(t, built, ifStmt, 2)[0], factflow.CallSiteContextCondition, 0, true, false, true, false)
	assertSite("iterator", requireStmtPoints(t, built, genericFor, 3)[0], factflow.CallSiteContextIteratorSource, 0, true, true, false, false)
	assertSite("return", requireStmtPoints(t, built, ret, 2)[0], factflow.CallSiteContextReturnSource, 0, true, true, false, true)
}

func TestLowerMemberCallLocalAssignmentUsesCallResultSource(t *testing.T) {
	makeCall := &ast.FuncCallExpr{
		Func: dot(ident("builder"), "build"),
	}
	local := localAssign([]string{"batch"}, makeCall)
	stmts := []ast.Stmt{local}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"builder"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	points := requireStmtPoints(t, built, local, 2)
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing member assignment call site")
	}
	if site.Context() != factflow.CallSiteContextAssignmentSource {
		t.Fatalf("member call context = %v, want assignment source", site.Context())
	}
	if _, ok := callproducer.FromFacts(facts, points[0]); !ok {
		t.Fatalf("member assignment call point %d missing producer", points[0])
	}
	assign, ok := facts.RootAssignment(points[1])
	if !ok {
		t.Fatalf("missing local assignment at point %d", points[1])
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceCall || source.CallPoint != points[0] || !source.HasCallPoint || source.ResultIndex != 0 {
		t.Fatalf("member assignment source = %#v, want call result 0 from point %d", source, points[0])
	}
}

func TestLowerCallSiteResultTargetPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = nil
    local other = nil
    value = make()
end
`, "make")
	assignStmt, ok := fn.Stmts[2].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want assignment", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, assignStmt, 2)
	callPoint := points[0]
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")

	body := wir.NewBody("synthetic-call-target-owner")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, start+1)
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetOrdinaryAssignment,
		Index:       0,
		ResultIndex: 0,
		Path:        otherPath,
	})

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 {
		t.Fatalf("call result targets = %#v, want one", targets)
	}
	if got := targets[0].TargetPath(); !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("call result target path = %v, want WIR path %v not semantic path %v", got, otherPath, valuePath)
	}
}

func TestLowerCallSiteResultTargetDoesNotFallbackToSemanticPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = nil
    value = make()
end
`, "make")
	assignStmt, ok := fn.Stmts[1].(*ast.AssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want assignment", fn.Stmts[1])
	}
	callPoint := requireStmtPoints(t, built, assignStmt, 2)[0]
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value")
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")

	body := wir.NewBody("synthetic-missing-call-target")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, start+1)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 0 {
		t.Fatalf("call result targets = %#v, want no semantic fallback target for %v", targets, valuePath)
	}
}

func TestLowerCallSiteResultTargetShapeComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = make()
end
`, "make")
	localStmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	callPoint := requireStmtPoints(t, built, localStmt, 2)[0]
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")

	body := wir.NewBody("synthetic-call-target-shape")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, start+1)
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetExpression,
		Index:       42,
		ResultIndex: 0,
	})

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 {
		t.Fatalf("call result targets = %#v, want one", targets)
	}
	if targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].Index() != 42 {
		t.Fatalf("call result target shape = %v/%d, want WIR expression/42", targets[0].Kind(), targets[0].Index())
	}
}

func TestLowerCallSiteResultTargetsCanComeOnlyFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    make()
end
`, "make")
	callStmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	callPoint := requireStmtPoints(t, built, callStmt, 1)[0]
	semanticCall, ok := result.Call(callPoint)
	if !ok {
		t.Fatalf("missing semantic call at point %d", callPoint)
	}
	if got := semanticCall.ResultTargets; len(got) != 0 {
		t.Fatalf("semantic call result targets = %#v, want none", got)
	}
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	makePath := path.NewPath(makeSym, "make")

	body := wir.NewBody("synthetic-wir-only-call-target")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpCall,
		Point:   callPoint,
		Call:    wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		Results: body.AppendOperands([]wir.Operand{resultTemp}),
	})
	body.SetPointRange(callPoint, start, start+1)
	body.SetCallResultTarget(callPoint, wir.CallResultTarget{
		Kind:        wir.CallResultTargetExpression,
		Index:       7,
		ResultIndex: 0,
	})

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 {
		t.Fatalf("call result targets = %#v, want WIR-only target", targets)
	}
	if targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].Index() != 7 || targets[0].ResultIndex() != 0 {
		t.Fatalf("call result target = %#v, want WIR expression index 7 result 0", targets[0])
	}
}

func TestLowerWIRCallSitePublishesWithoutSemanticCallView(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(payload: {id: string}): ()
    local marker = true
end
`, "make")
	localStmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, localStmt, 1)[0]
	if _, ok := result.CallView(point); ok {
		t.Fatalf("point %d unexpectedly has semantic call view", point)
	}
	makeSym, ok := bindings.GlobalSymbol("make")
	if !ok {
		t.Fatal("missing make global symbol")
	}
	payloadSym := bindings.ParamSlots(fn)[0].Symbol
	makePath := path.NewPath(makeSym, "make")
	payloadID := path.NewPath(payloadSym, "payload").Field("id")

	body := wir.NewBody("synthetic-call-without-semantic-view")
	resultTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: point,
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(makePath))}},
		List: body.AppendOperands([]wir.Operand{
			{Kind: wir.OperandPath, Ref: uint32(body.InternPath(payloadID))},
			{Kind: wir.OperandConst, Ref: uint32(body.InternConst(wir.Const{Kind: wir.ConstString, Str: "tag"}))},
		}),
		Results:     body.AppendOperands([]wir.Operand{resultTemp}),
		CallContext: wir.CallContextAssignmentSource,
		CallExpr:    3,
		CallFinal:   true,
		CallSpan:    source.Span{StartLine: 9, StartCol: 3, EndLine: 9, EndCol: 24},
		CalleeSpan:  source.Span{StartLine: 9, StartCol: 3, EndLine: 9, EndCol: 7},
		CallArgs: body.AppendCallArgumentMeta([]wir.CallArgumentMeta{
			{Span: source.Span{StartLine: 9, StartCol: 8, EndLine: 9, EndCol: 18}, Label: "payload.id"},
			{Span: source.Span{StartLine: 9, StartCol: 20, EndLine: 9, EndCol: 24}, Label: "\"tag\""},
		}),
		ExprID: 99,
	})
	body.SetPointRange(point, start, start+1)
	body.SetCallResultTarget(point, wir.CallResultTarget{
		Kind:        wir.CallResultTargetExpression,
		Index:       2,
		ResultIndex: 0,
	})

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(point)
	if !ok {
		t.Fatalf("missing WIR call site at point %d without semantic CallView", point)
	}
	if site.Context() != factflow.CallSiteContextAssignmentSource || site.ExprIndex() != 3 || !site.Final() {
		t.Fatalf("WIR call site flags = context:%v expr:%d final:%v", site.Context(), site.ExprIndex(), site.Final())
	}
	if got := site.CalleePath(); !got.Equal(makePath) {
		t.Fatalf("WIR call callee path = %v, want %v", got, makePath)
	}
	args := site.ArgumentSources()
	if len(args) != 2 {
		t.Fatalf("WIR call args = %#v, want two", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("first WIR call arg = %#v, want expression source", args[0])
	}
	gotPath, ok := facts.ExpressionPath(args[0].ExprRef)
	if !ok || !gotPath.Equal(payloadID) {
		t.Fatalf("first WIR call arg path = %v/%v, want %v", gotPath, ok, payloadID)
	}
	if args[1].Kind != factflow.ValueSourceLiteral || args[1].LiteralKind != factflow.ValueSourceLiteralString || args[1].String != "tag" {
		t.Fatalf("second WIR call arg = %#v, want string literal tag", args[1])
	}
	firstSpan, ok := site.ArgumentSpanAt(0)
	if !ok || firstSpan.StartCol != 8 {
		t.Fatalf("first WIR call argument span = %#v/%v", firstSpan, ok)
	}
	secondSpan, ok := site.ArgumentSpanAt(1)
	if !ok || secondSpan.StartCol != 20 {
		t.Fatalf("second WIR call argument span = %#v/%v", secondSpan, ok)
	}
	firstLabel, ok := site.ArgumentLabelAt(0)
	if !ok || firstLabel != "payload.id" {
		t.Fatalf("first WIR call argument label = %q/%v", firstLabel, ok)
	}
	secondLabel, ok := site.ArgumentLabelAt(1)
	if !ok || secondLabel != "\"tag\"" {
		t.Fatalf("second WIR call argument label = %q/%v", secondLabel, ok)
	}
	if targets := site.ResultTargets(); len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].Index() != 2 {
		t.Fatalf("WIR call targets = %#v, want expression target index 2", targets)
	}
}

func TestLowerReturnCallResultTargetComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): string
    return make()
end
`, "make")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	callPoint := requireStmtPoints(t, built, ret, 2)[0]
	body := wirlower.Lower("return-target", fn.Stmts, bindings, built)

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetReturn || targets[0].Index() != 0 || targets[0].ResultIndex() != 0 {
		t.Fatalf("return call targets = %#v, want WIR return index 0 result 0", targets)
	}
}

func TestLowerWithWIRPublishesCallAndReturnWithoutSemanticSidecars(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function f(payload: { id: string }): string
    return make(payload.id)
end
`, "make")
	ret, ok := fn.Stmts[0].(*ast.ReturnStmt)
	if !ok {
		t.Fatalf("stmt = %T, want return", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, ret, 2)
	callPoint := points[0]
	returnPoint := points[1]
	body := wirlower.Lower("no-sidecars", fn.Stmts, bindings, built)

	facts := Lower(nil, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	if got := site.Context(); got != factflow.CallSiteContextReturnSource {
		t.Fatalf("WIR call context = %v, want return source", got)
	}
	targets := site.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetReturn || targets[0].Index() != 0 || targets[0].ResultIndex() != 0 {
		t.Fatalf("return call targets = %#v, want WIR return index 0 result 0", targets)
	}
	retFact, ok := facts.Return(returnPoint)
	if !ok {
		t.Fatalf("missing WIR return fact at point %d", returnPoint)
	}
	sources := retFact.Sources()
	if len(sources) != 1 {
		t.Fatalf("return sources = %#v, want one source", sources)
	}
	if source := sources[0]; source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != callPoint || source.ResultIndex != 0 {
		t.Fatalf("return source = %#v, want call result from point %d", source, callPoint)
	}
}

func TestLowerNegatedConditionCallSiteCarriesPolarity(t *testing.T) {
	readyCall := &ast.FuncCallExpr{Func: ident("ready")}
	ifStmt := &ast.IfStmt{Condition: &ast.UnaryNotOpExpr{Expr: readyCall}}
	stmts := []ast.Stmt{ifStmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"ready"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	points := requireStmtPoints(t, built, ifStmt, 2)
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing condition call site")
	}
	if site.Context() != factflow.CallSiteContextCondition || !site.ConditionNegated() {
		t.Fatalf("condition call site = context %v negated=%v, want negated condition", site.Context(), site.ConditionNegated())
	}

	body := wirlower.Lower("negated-condition", stmts, bindings, built)
	wirFacts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	wirSite, ok := wirFacts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR condition call site")
	}
	if wirSite.Context() != factflow.CallSiteContextCondition || !wirSite.ConditionNegated() {
		t.Fatalf("WIR condition call site = context %v negated=%v, want negated condition", wirSite.Context(), wirSite.ConditionNegated())
	}
}

func TestLowerCallSitePreservesPortableCallShapeAndArgumentOverlays(t *testing.T) {
	obj := ident("obj")
	arg := ident("arg")
	other := ident("other")
	castArg := &ast.CastExpr{
		Expr:   arg,
		Type:   primitiveType("number"),
		Syntax: ast.CastSyntaxAs,
	}
	call := &ast.FuncCallExpr{
		Receiver: obj,
		Method:   "run",
		Args:     []ast.Expr{castArg, other},
		TypeArgs: []ast.TypeExpr{primitiveType("string"), primitiveType("number")},
	}
	stmt := &ast.FuncCallStmt{Expr: call}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"obj", "arg", "other"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})
	assertNoCompilerASTTypes(t, reflect.TypeOf(facts))

	point := requireStmtPoints(t, built, stmt, 1)[0]
	site, ok := facts.CallSite(point)
	if !ok {
		t.Fatalf("missing call site at point %d", point)
	}
	if site.Context() != factflow.CallSiteContextStatement || site.MethodName() != "run" {
		t.Fatalf("call site context/method = %v/%q", site.Context(), site.MethodName())
	}
	receiverPath := path.NewPath(mustIdentSymbol(t, bindings, obj), "obj")
	methodPath := receiverPath.Field("run")
	if got, ok := site.ReceiverPath(); !ok || !got.Equal(receiverPath) {
		t.Fatalf("receiver path = %#v/%v, want %#v/true", got, ok, receiverPath)
	}
	if got, ok := site.MethodPath(); !ok || !got.Equal(methodPath) {
		t.Fatalf("method path = %#v/%v, want %#v/true", got, ok, methodPath)
	}
	if got := site.CalleePath(); !got.Equal(methodPath) {
		t.Fatalf("callee path = %#v, want %#v", got, methodPath)
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceExpression || !receiverSource.HasExpr || receiverSource.ExprRef == 0 {
		t.Fatalf("receiver source = %#v/%v, want lowered expression source", receiverSource, ok)
	}

	args := site.ArgumentSources()
	if len(args) != 2 {
		t.Fatalf("argument sources = %#v, want two args", args)
	}
	if args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr || args[0].ExprRef == 0 || args[0].ExprIndex != 0 || args[0].TargetIndex != 0 || args[0].Final {
		t.Fatalf("first arg source = %#v", args[0])
	}
	if args[1].Kind != factflow.ValueSourceExpression || !args[1].HasExpr || args[1].ExprRef == 0 || args[1].ExprIndex != 1 || args[1].TargetIndex != 1 || !args[1].Final {
		t.Fatalf("second arg source = %#v", args[1])
	}
	assertLoweredAssertion(t, facts, args[0], concreteCastAssertionForType(typ.Number), factflow.ValueSourceExpression)

	typeArgs := site.TypeArgs()
	if len(typeArgs) != 2 || typeArgs[0] == 0 || typeArgs[1] == 0 || typeArgs[0] == typeArgs[1] {
		t.Fatalf("type args = %#v, want two distinct opaque refs", typeArgs)
	}
	if targets := site.ResultTargets(); len(targets) != 0 {
		t.Fatalf("statement method call targets = %#v, want none", targets)
	}
	if _, ok := callproducer.FromFacts(facts, point); ok {
		t.Fatalf("statement method call point %d lowered as call producer", point)
	}

	producerType := reflect.TypeOf(factflow.CallProducer{})
	for _, method := range []string{"ReceiverPath", "MethodPath", "MethodName", "ArgumentSources", "TypeArgs"} {
		if _, ok := producerType.MethodByName(method); ok {
			t.Fatalf("CallProducer unexpectedly exposes broad call-shape method %s", method)
		}
	}
}

func TestLowerCallSiteUsesSemanticArgumentSources(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	wrapped := &ast.CastExpr{
		Expr:   inner,
		Type:   primitiveType("number"),
		Syntax: ast.CastSyntaxAs,
	}
	outer := &ast.FuncCallExpr{
		Func: ident("f"),
		Args: []ast.Expr{wrapped},
	}
	innerPoint := cfg.Point(42)
	l := lowerer{
		exprs: make(map[any]factflow.ExprRef),
	}
	semanticSource := sourceprovenance.SourceForExpr(wrapped, 0, 0, 0, true, false, func(exprIndex int, call *ast.FuncCallExpr) (cfg.Point, bool) {
		return innerPoint, exprIndex == 0 && call == inner
	})
	site := l.callSite(semantics.CallFact{
		Context:         semantics.CallContextStatement,
		Call:            outer,
		Args:            []ast.Expr{ident("not_semantic_source")},
		ArgumentSources: []sourceprovenance.ASTSource{semanticSource},
		ArgumentSpans: []semantics.SourceSpan{
			{StartLine: 7, StartCol: 2, EndLine: 7, EndCol: 18},
		},
		ArgumentLabels: []string{"wrapped_call"},
	})
	args := site.ArgumentSources()
	if len(args) != 1 {
		t.Fatalf("argument sources = %#v, want one arg", args)
	}
	arg := args[0]
	if arg.Kind != factflow.ValueSourceCall || !arg.HasCallPoint || arg.CallPoint != innerPoint {
		t.Fatalf("nested call arg source = %#v, want call point %d", arg, innerPoint)
	}
	if span, ok := site.ArgumentSpanAt(0); !ok || span.StartLine != 7 || span.EndCol != 18 {
		t.Fatalf("argument span = %#v/%v, want lowered semantic span", span, ok)
	}
	if label, ok := site.ArgumentLabelAt(0); !ok || label != "wrapped_call" {
		t.Fatalf("argument label = %q/%v, want wrapped_call/true", label, ok)
	}
}

func TestLowerCallSiteUsesWIRArgumentSourcesForLiteralAndCallOperands(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value, "ok", 3, nil, true, produce())
end
`, "send", "produce")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 2)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	site, ok := facts.CallSite(points[1])
	if !ok {
		t.Fatalf("missing outer call site at point %d", points[1])
	}
	args := site.ArgumentSources()
	if len(args) != 6 {
		t.Fatalf("argument sources = %#v, want six", args)
	}
	wantParamKey := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value").Key()
	if args[0].Kind != factflow.ValueSourcePath || args[0].PathKey != wantParamKey || args[0].HasExpr {
		t.Fatalf("param argument source = %#v, want WIR path source %q", args[0], wantParamKey)
	}
	if args[1].Kind != factflow.ValueSourceLiteral || args[1].LiteralKind != factflow.ValueSourceLiteralString || args[1].String != "ok" || args[1].HasExpr {
		t.Fatalf("string argument source = %#v, want WIR string literal", args[1])
	}
	if args[2].Kind != factflow.ValueSourceLiteral || args[2].LiteralKind != factflow.ValueSourceLiteralInteger || args[2].Int != 3 || args[2].HasExpr {
		t.Fatalf("integer argument source = %#v, want WIR integer literal", args[2])
	}
	if args[3].Kind != factflow.ValueSourceNil {
		t.Fatalf("nil argument source = %#v", args[3])
	}
	if args[4].Kind != factflow.ValueSourceLiteral || args[4].LiteralKind != factflow.ValueSourceLiteralBool || !args[4].Bool || args[4].HasExpr {
		t.Fatalf("bool argument source = %#v, want WIR bool literal", args[4])
	}
	if args[5].Kind != factflow.ValueSourceCall || !args[5].HasCallPoint || args[5].CallPoint != points[0] ||
		!args[5].Final || !args[5].Expanded || args[5].Adjusted || args[5].OpenTail {
		t.Fatalf("tail call argument source = %#v, want expanded non-open call from point %d", args[5], points[0])
	}
}

func TestLowerCallSiteUsesWIRArgumentSourceForLocalRootPath(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = "x"
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[1].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[1])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})

	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok {
		t.Fatalf("missing first argument source")
	}
	want := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Key()
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr || arg.PathKey != "" {
		t.Fatalf("local argument source = %#v, want WIR expression-backed path source", arg)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || got.Key() != want {
		t.Fatalf("local argument expression path = %v/%v, want %q", got, ok, want)
	}
}

func TestLowerCallSiteLocalArgumentPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = "x"
    local other = "y"
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valueStmt := fn.Stmts[0].(*ast.LocalAssignStmt)
	otherStmt := fn.Stmts[1].(*ast.LocalAssignStmt)
	valuePath := path.NewPath(mustLocalAt(t, bindings, valueStmt, 0), "value")
	otherPath := path.NewPath(mustLocalAt(t, bindings, otherStmt, 0), "other")
	body := wir.NewBody("synthetic-call")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want expression-backed WIR path", arg, ok)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("argument expression path = %v/%v, want WIR path %v not semantic path %v", got, ok, otherPath, valuePath)
	}
}

func TestLowerCallSiteSegmentedArgumentPathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local value = { name = "x" }
    local other = { name = "y" }
    send(value.name)
end
`, "send")
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valuePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "value").Field("name")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("name")
	body := wir.NewBody("synthetic-segmented-call-arg")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want expression-backed WIR path", arg, ok)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok || !got.Equal(otherPath) || got.Equal(valuePath) {
		t.Fatalf("argument expression path = %v/%v, want WIR path %v not semantic path %v", got, ok, otherPath, valuePath)
	}
}

func TestLowerDirectRootCallShapeComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local callee = function() end
    local other = function() end
    callee()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	calleePath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "callee")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-direct-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if got := site.CalleeSymbol(); got != otherPath.Symbol || got == calleePath.Symbol {
		t.Fatalf("callee symbol = %d, want WIR symbol %d not semantic symbol %d", got, otherPath.Symbol, calleePath.Symbol)
	}
	if got := site.CalleePath(); !got.Equal(otherPath) || got.Equal(calleePath) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", got, otherPath, calleePath)
	}
	if site.CalleeMemberAccess() {
		t.Fatalf("direct root call unexpectedly marked as member access")
	}
}

func TestLowerDirectMemberCallShapeComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function() end }
    local other = { run = function() end }
    obj.run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-direct-member-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath.Field("run")))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if got := site.CalleeSymbol(); got != otherPath.Symbol || got == objPath.Symbol {
		t.Fatalf("callee symbol = %d, want WIR symbol %d not semantic symbol %d", got, otherPath.Symbol, objPath.Symbol)
	}
	if got := site.CalleePath(); !got.Equal(otherPath.Field("run")) || got.Equal(objPath.Field("run")) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", got, otherPath.Field("run"), objPath.Field("run"))
	}
	receiver, member, ok := site.CalleeMemberAccessPath()
	if !ok || !receiver.Equal(otherPath) || receiver.Equal(objPath) || member.Name != "run" {
		t.Fatalf("member access path = %v/%#v/%v, want WIR receiver %v .run not semantic receiver %v", receiver, member, ok, otherPath, objPath)
	}
}

func TestLowerMethodCallReceiverSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function(self) end }
    local other = { run = function(self) end }
    obj:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-method-receiver")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourcePath || receiverSource.PathKey != otherPath.Key() {
		t.Fatalf("receiver source = %#v/%v, want WIR path source %v", receiverSource, ok, otherPath)
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || !receiverPath.Equal(otherPath) || receiverPath.Equal(objPath) {
		t.Fatalf("receiver path = %v/%v, want WIR path %v not semantic path %v", receiverPath, ok, otherPath, objPath)
	}
	methodPath, ok := site.MethodPath()
	if !ok || !methodPath.Equal(otherPath.Field("run")) || methodPath.Equal(objPath.Field("run")) {
		t.Fatalf("method path = %v/%v, want WIR path %v not semantic path %v", methodPath, ok, otherPath.Field("run"), objPath.Field("run"))
	}
	if calleePath := site.CalleePath(); !calleePath.Equal(otherPath.Field("run")) || calleePath.Equal(objPath.Field("run")) {
		t.Fatalf("callee path = %v, want WIR path %v not semantic path %v", calleePath, otherPath.Field("run"), objPath.Field("run"))
	}
}

func TestLowerMethodCallSegmentedReceiverSourcePathComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { child = { run = function(self) end } }
    local other = { child = { run = function(self) end } }
    obj.child:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[0].(*ast.LocalAssignStmt), 0), "obj").Field("child")
	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other").Field("child")
	body := wir.NewBody("synthetic-segmented-method-receiver")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceExpression || !receiverSource.HasExpr {
		t.Fatalf("receiver source = %#v/%v, want expression-backed WIR path", receiverSource, ok)
	}
	gotPath, ok := facts.ExpressionPath(receiverSource.ExprRef)
	if !ok || !gotPath.Equal(otherPath) || gotPath.Equal(objPath) {
		t.Fatalf("receiver source path = %v/%v, want WIR path %v not semantic path %v", gotPath, ok, otherPath, objPath)
	}
	receiverPath, ok := site.ReceiverPath()
	if !ok || !receiverPath.Equal(otherPath) || receiverPath.Equal(objPath) {
		t.Fatalf("receiver path = %v/%v, want WIR path %v not semantic path %v", receiverPath, ok, otherPath, objPath)
	}
}

func TestLowerMethodCallReceiverSourceDoesNotRequireSemanticReceiverSource(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local obj = { run = function(self) end }
    local other = { run = function(self) end }
    obj:run()
end
`)
	stmt, ok := fn.Stmts[2].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[2])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	fact, ok := result.Call(points[0])
	if !ok {
		t.Fatalf("missing semantic call fact at point %d", points[0])
	}
	fact.HasReceiverSource = false
	fact.ReceiverSource = sourceprovenance.ASTSource{}

	otherPath := path.NewPath(mustLocalAt(t, bindings, fn.Stmts[1].(*ast.LocalAssignStmt), 0), "other")
	body := wir.NewBody("synthetic-method-receiver-no-sidecar")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(otherPath))},
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	l := lowerer{
		bindings:        bindings,
		wir:             body,
		exprs:           make(map[any]factflow.ExprRef),
		expressionPaths: make(map[factflow.ExprRef]path.Path),
	}
	site := l.callSiteWithArgumentSourcesAt(points[0], fact, nil)
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourcePath || receiverSource.PathKey != otherPath.Key() {
		t.Fatalf("receiver source = %#v/%v, want WIR path source %v without semantic receiver source", receiverSource, ok, otherPath)
	}
}

func TestLowerCallSiteDoesNotFallbackWhenWIRCallInstructionMissing(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: wir.NewBody("empty")})

	if _, ok := facts.CallSite(points[0]); ok {
		t.Fatalf("WIR mode call at point %d fell back to semantic sidecar", points[0])
	}
}

func TestLowerCallSiteContextAndFlagsComeFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    send()
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.Call(point)
	if !ok {
		t.Fatalf("missing semantic call at point %d", point)
	}
	if fact.Context != semantics.CallContextStatement {
		t.Fatalf("semantic context = %v, want statement", fact.Context)
	}
	sendSym, ok := bindings.GlobalSymbol("send")
	if !ok {
		t.Fatal("missing send global symbol")
	}
	sendPath := path.NewPath(sendSym, "send")
	body := wir.NewBody("synthetic-call-context")
	start := body.Emit(wir.Instruction{
		Op:           wir.OpCall,
		Point:        point,
		Call:         wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(sendPath))}},
		CallContext:  wir.CallContextReturnSource,
		CallExpr:     9,
		CallFinal:    true,
		CallExpanded: true,
		CallAdjusted: false,
		CallOpenTail: true,
	})
	body.SetPointRange(point, start, start+1)

	l := lowerer{
		bindings: bindings,
		wir:      body,
		exprs:    make(map[any]factflow.ExprRef),
	}
	site := l.callSiteWithArgumentSourcesAt(point, fact, nil)
	if site.Context() != factflow.CallSiteContextReturnSource || site.ExprIndex() != 9 {
		t.Fatalf("call site context/index = %v/%d, want WIR return-source/9", site.Context(), site.ExprIndex())
	}
	if !site.Final() || !site.Expanded() || site.Adjusted() || !site.OpenTail() {
		t.Fatalf("call site flags = final:%v expanded:%v adjusted:%v open:%v, want WIR true/true/false/true", site.Final(), site.Expanded(), site.Adjusted(), site.OpenTail())
	}
}

func TestLowerCallSiteMetadataComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	point := requireStmtPoints(t, built, stmt, 1)[0]
	fact, ok := result.Call(point)
	if !ok {
		t.Fatalf("missing semantic call at point %d", point)
	}
	fact.CallSpan = semantics.SourceSpan{StartLine: 40, StartCol: 1, EndLine: 40, EndCol: 2}
	fact.CalleeSpan = semantics.SourceSpan{StartLine: 41, StartCol: 1, EndLine: 41, EndCol: 2}
	fact.ArgumentSpans = []semantics.SourceSpan{{StartLine: 42, StartCol: 1, EndLine: 42, EndCol: 2}}
	fact.ArgumentLabels = []string{"semantic_value"}

	body := wir.NewBody("synthetic-call-metadata")
	meta := []wir.CallArgumentMeta{{
		Span:  source.Span{StartLine: 7, StartCol: 11, EndLine: 7, EndCol: 16},
		Label: "wir_value",
	}}
	start := body.Emit(wir.Instruction{
		Op:         wir.OpCall,
		Point:      point,
		CallSpan:   source.Span{StartLine: 7, StartCol: 5, EndLine: 7, EndCol: 17},
		CalleeSpan: source.Span{StartLine: 7, StartCol: 5, EndLine: 7, EndCol: 9},
		CallArgs:   body.AppendCallArgumentMeta(meta),
	})
	body.SetPointRange(point, start, start+1)

	l := lowerer{
		bindings: bindings,
		wir:      body,
		exprs:    make(map[any]factflow.ExprRef),
	}
	site := l.callSiteWithArgumentSourcesAt(point, fact, nil)
	if got := site.CallSpan(); got.StartLine != 7 || got.StartCol != 5 || got.EndCol != 17 {
		t.Fatalf("call span = %#v, want WIR span", got)
	}
	if got := site.CalleeSpan(); got.StartLine != 7 || got.StartCol != 5 || got.EndCol != 9 {
		t.Fatalf("callee span = %#v, want WIR span", got)
	}
	if got, ok := site.ArgumentSpanAt(0); !ok || got.StartLine != 7 || got.StartCol != 11 || got.EndCol != 16 {
		t.Fatalf("argument span = %#v/%v, want WIR argument span", got, ok)
	}
	if got, ok := site.ArgumentLabelAt(0); !ok || got != "wir_value" {
		t.Fatalf("argument label = %q/%v, want WIR label", got, ok)
	}
}

func TestLowerCallSiteUsesUnknownForUnsupportedDirectWIRArgument(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-unsupported-call-arg")
	unsupported := wir.Operand{Kind: wir.OperandType, Ref: uint32(body.InternType(typ.String))}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{unsupported}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceUnknown || args[0].TargetIndex != 0 {
		t.Fatalf("WIR call argument sources = %#v, want one unknown source for unsupported direct operand", args)
	}
}

func TestLowerCallSiteUsesUnknownForMissingTempWIRArgument(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-missing-temp-call-arg")
	unsupported := wir.Operand{Kind: wir.OperandTemp, Ref: 99}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{unsupported}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceUnknown || args[0].TargetIndex != 0 {
		t.Fatalf("WIR call argument sources = %#v, want one unknown source for missing temp operand", args)
	}
}

func TestLowerCallSiteUsesUnknownForUnsupportedDefinedTempWIRArgument(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
	body := wir.NewBody("synthetic-unsupported-defined-temp-call-arg")
	temp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:      wir.OpIterate,
		Point:   points[0],
		Iter:    wir.IterGeneric,
		Results: body.AppendOperands([]wir.Operand{temp}),
		List:    body.AppendOperands([]wir.Operand{{Kind: wir.OperandNone}}),
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{temp}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceUnknown || args[0].TargetIndex != 0 {
		t.Fatalf("WIR call argument sources = %#v, want unknown without falling back to semantic %s", args, valuePath.Key())
	}
	if gotPath, ok := facts.ExpressionPath(args[0].ExprRef); ok && gotPath.Equal(valuePath) {
		t.Fatalf("WIR call argument fell back to semantic path %v", gotPath)
	}
}

func TestLowerCallSiteLogicalFallbackArgumentComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(env: {get: fun(string): string?}, config: {DEFAULT_CACHE_ID: string}, store: {get: fun(string): any}): ()
    store.get(env.get("APP_CACHE") or config.DEFAULT_CACHE_ID)
end
`)
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 2)
	callPoint := points[1]
	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	reg := standard.Registry()
	facts := Lower(result, built.Graph, Config{Registry: reg, Bindings: bindings, WIR: body})

	site, ok := facts.CallSite(callPoint)
	if !ok {
		t.Fatalf("missing WIR call site at point %d", callPoint)
	}
	args := site.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceExpression || !args[0].HasExpr {
		t.Fatalf("WIR logical call argument source = %#v, want expression source", args)
	}
	op, ok := facts.ExpressionOperation(args[0].ExprRef)
	if !ok || op.Kind() != factflow.ExpressionOperationBinary || op.Op() != "or" {
		t.Fatalf("WIR logical argument operation = %#v/%v, want or", op, ok)
	}
	if left := op.Left(); left.Kind != factflow.ValueSourceCall || !left.HasCallPoint {
		t.Fatalf("WIR logical argument left source = %#v, want env.get call result", left)
	}
	right := op.Right()
	if right.Kind != factflow.ValueSourceExpression || !right.HasExpr {
		t.Fatalf("WIR logical argument right source = %#v, want path expression", right)
	}
	value, ok := facts.ExpressionValue(right.ExprRef)
	if !ok {
		t.Fatalf("missing WIR logical argument fallback value for ref %d", right.ExprRef)
	}
	got, ok := typevalue.TypeOf(reg, value)
	if !ok || !typ.TypeEquals(got, typ.String) {
		t.Fatalf("WIR logical argument fallback value type = %v/%v, want string", got, ok)
	}
}

func TestLowerAssignmentSelectResultSourceComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local selected = "fallback"
end
`)
	stmt, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	selectedPath := path.NewPath(mustLocalAt(t, bindings, stmt, 0), "selected")
	body := wir.NewBody("synthetic-select-result-source")
	temp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	start := body.Emit(wir.Instruction{
		Op:            wir.OpSelect,
		Point:         points[0],
		Dst:           temp,
		SelectDefault: true,
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpAssign,
		Point: points[0],
		Dst:   wir.Operand{Kind: wir.OperandPath, Ref: uint32(body.InternPath(selectedPath))},
		A:     temp,
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	assign, ok := facts.RootAssignment(points[0])
	if !ok {
		t.Fatalf("missing assignment at point %d", points[0])
	}
	source := assign.Source()
	if source.Kind != factflow.ValueSourceCall || source.CallPoint != points[0] || !source.HasCallPoint || source.ResultIndex != 0 {
		t.Fatalf("assignment source = %#v, want WIR select-result source", source)
	}
}

func TestLowerCallSiteClosureArgumentFunctionComesFromWIR(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): ()
    local other = function() end
    send(function() end)
end
`, "send")
	stmt, ok := fn.Stmts[1].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[1])
	}
	callExpr, ok := stmt.Expr.(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("call expr = %T, want function call", stmt.Expr)
	}
	callArg, ok := callExpr.Args[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("argument = %T, want function expression", callExpr.Args[0])
	}
	actualSym, ok := bindings.FunctionSymbol(callArg)
	if !ok || actualSym == 0 {
		t.Fatalf("actual argument function symbol = %d/%v", actualSym, ok)
	}
	otherAssign, ok := fn.Stmts[0].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("stmt = %T, want local assignment", fn.Stmts[0])
	}
	otherFn, ok := otherAssign.Exprs[0].(*ast.FunctionExpr)
	if !ok {
		t.Fatalf("other expr = %T, want function expression", otherAssign.Exprs[0])
	}
	otherSym, ok := bindings.FunctionSymbol(otherFn)
	if !ok || otherSym == 0 || otherSym == actualSym {
		t.Fatalf("other function symbol = %d/%v, actual=%d", otherSym, ok, actualSym)
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-closure-call-arg")
	closureTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 1}
	proto := body.AddProto(wir.FuncProto{Name: "other", Symbol: otherSym})
	start := body.Emit(wir.Instruction{
		Op:    wir.OpClosure,
		Point: points[0],
		Dst:   closureTemp,
		Func:  proto,
	})
	body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		List:  body.AppendOperands([]wir.Operand{closureTemp}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok || arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("argument source = %#v/%v, want WIR closure expression source", arg, ok)
	}
	gotSym, ok := facts.ExpressionFunction(arg.ExprRef)
	if !ok || gotSym != otherSym || gotSym == actualSym {
		t.Fatalf("expression function = %d/%v, want WIR proto symbol %d not semantic argument symbol %d", gotSym, ok, otherSym, actualSym)
	}
}

func TestLowerCallSiteDoesNotFallbackToSemanticCalleeWhenWIRDirectCalleeUnsupported(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(value: string): ()
    send(value)
end
`, "send")
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	valuePath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "value")
	body := wir.NewBody("synthetic-unsupported-call-callee")
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call:  wir.CallInfo{Callee: wir.Operand{Kind: wir.OperandTemp, Ref: 7}},
		List:  body.AppendOperands([]wir.Operand{{Kind: wir.OperandPath, Ref: uint32(body.InternPath(valuePath))}}),
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	if site.CalleeSymbol() != 0 || !site.CalleePath().IsEmpty() || site.CalleeMemberAccess() {
		t.Fatalf("WIR unsupported callee kept semantic callee identity: symbol=%d path=%s member=%v", site.CalleeSymbol(), site.CalleePath(), site.CalleeMemberAccess())
	}
}

func TestLowerMethodCallNameComesFromWIRForExpressionReceiver(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(obj: any): ()
    obj:run()
end
`)
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	body := wir.NewBody("synthetic-expression-receiver-method")
	receiverTemp := wir.Operand{Kind: wir.OperandTemp, Ref: 9}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "stop"}),
			Receiver: receiverTemp,
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	if !site.CalleeMemberAccess() || site.MethodName() != "stop" {
		t.Fatalf("method call shape = member:%v method:%q, want WIR method stop", site.CalleeMemberAccess(), site.MethodName())
	}
	if _, ok := site.ReceiverPath(); ok {
		t.Fatalf("expression receiver unexpectedly kept semantic receiver path")
	}
	if _, ok := site.MethodPath(); ok {
		t.Fatalf("expression receiver unexpectedly kept semantic method path")
	}
}

func TestLowerMethodCallExpressionReceiverSourceDoesNotRequireSemanticReceiverSource(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(): string
    local function make(): {{topic: (self: any) -> string}}
        return {}
    end
    return make()[1]:topic()
end
`)
	var callPoint cfg.Point
	var fact semantics.CallFact
	for _, point := range built.Graph.RPO() {
		candidate, ok := result.Call(point)
		if ok && candidate.Method == "topic" {
			callPoint = point
			fact = candidate
			break
		}
	}
	if callPoint == 0 {
		t.Fatalf("missing topic call fact")
	}
	fact.HasReceiverSource = false
	fact.ReceiverSource = sourceprovenance.ASTSource{}

	body := wirlower.Lower("f", fn.Stmts, bindings, built)
	l := lowerer{
		bindings:                      bindings,
		graph:                         built.Graph,
		graphID:                       built.Graph.ID(),
		typeResolver:                  typeresolve.New(bindings),
		typeValues:                    typevalue.NewCache(),
		wir:                           body,
		callPoints:                    callPointsByExpr(builtCallFacts(built.Graph, result)),
		symbolTypes:                   lowerSymbolTypes(bindings, built.Graph, built.Meta, result, typeresolve.New(bindings), importlookup.Source{}),
		exprs:                         make(map[any]factflow.ExprRef),
		types:                         make(map[any]factflow.TypeRef),
		expressionValues:              make(map[factflow.ExprRef]product.Value),
		expressionOperations:          make(map[factflow.ExprRef]factflow.ExpressionOperation),
		expressionFunctions:           make(map[factflow.ExprRef]symbol.ID),
		expressionPaths:               make(map[factflow.ExprRef]path.Path),
		dynamicIndexExpressions:       make(map[factflow.ExprRef]factflow.DynamicIndexExpression),
		expressionConditions:          make(map[factflow.ExprRef]factflow.ExpressionCondition),
		expressionRefinements:         make(map[factflow.ExprRef]factflow.ExpressionRefinement),
		localConditionAliases:         make(map[symbol.ID]factflow.ExpressionCondition),
		wirTempDefinitions:            nil,
		wirTempDefinitionSets:         nil,
		wirStaticReachable:            nil,
		wirReachability:               nil,
		declaredReturnLocalTypes:      nil,
		returnLocalObjectLiteralTypes: nil,
	}
	site := l.callSiteWithArgumentSourcesAt(callPoint, fact, nil)
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceExpression || !receiverSource.HasExpr || receiverSource.ExprRef == 0 {
		t.Fatalf("receiver source = %#v/%v, want WIR expression source without semantic receiver source", receiverSource, ok)
	}
	if _, ok := site.ReceiverPath(); ok {
		t.Fatalf("expression receiver unexpectedly has receiver path")
	}
}

func TestLowerMethodCallUsesUnknownForUnsupportedWIRReceiver(t *testing.T) {
	fn, bindings, built, result := parseSemanticFunction(t, `
function f(obj: {run: fun(self: any): ()}): ()
    obj:run()
end
`)
	stmt, ok := fn.Stmts[0].(*ast.FuncCallStmt)
	if !ok {
		t.Fatalf("stmt = %T, want call statement", fn.Stmts[0])
	}
	points := requireStmtPoints(t, built, stmt, 1)
	objPath := path.NewPath(bindings.ParamSlots(fn)[0].Symbol, "obj")
	body := wir.NewBody("synthetic-unsupported-method-receiver")
	receiver := wir.Operand{Kind: wir.OperandTemp, Ref: 99}
	start := body.Emit(wir.Instruction{
		Op:    wir.OpCall,
		Point: points[0],
		Call: wir.CallInfo{
			Method:   body.InternConst(wir.Const{Kind: wir.ConstString, Str: "run"}),
			Receiver: receiver,
		},
	})
	body.SetPointRange(points[0], start, body.Len())

	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing WIR call site at point %d", points[0])
	}
	receiverSource, ok := site.ReceiverSource()
	if !ok || receiverSource.Kind != factflow.ValueSourceUnknown {
		t.Fatalf("receiver source = %#v/%v, want unknown without semantic fallback", receiverSource, ok)
	}
	if receiverSource.HasExpr {
		if gotPath, ok := facts.ExpressionPath(receiverSource.ExprRef); ok && gotPath.Equal(objPath) {
			t.Fatalf("receiver source fell back to semantic path %v", gotPath)
		}
	}
}

func TestLowerCallSitePreservesMemberAccessEvidence(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	site := l.callSite(semantics.CallFact{
		Context:            semantics.CallContextStatement,
		Call:               &ast.FuncCallExpr{Func: ident("make")},
		CalleePath:         path.Path{Root: "api"}.Field("make"),
		HasCalleePath:      true,
		CalleeMemberAccess: true,
	})
	if !site.CalleeMemberAccess() {
		t.Fatalf("lowered call site dropped member-access evidence")
	}
}

func TestLowerIteratorCallSitePreservesArgumentPath(t *testing.T) {
	stmts, bindings, built, result := parseSemanticChunk(t, `
local state = {
    active_sessions = {},
}

for id, session_info in pairs(state.active_sessions) do
end
`, "pairs")
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings})

	genericFor, ok := stmts[1].(*ast.GenericForStmt)
	if !ok {
		t.Fatalf("statement 1 = %T, want *ast.GenericForStmt", stmts[1])
	}
	points := built.StmtPoints.PointsFor(genericFor)
	if len(points) == 0 {
		t.Fatalf("generic-for points = %v, want iterator call point", points)
	}
	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing iterator call site at point %d", points[0])
	}
	arg, ok := site.ArgumentSourceAt(0)
	if !ok {
		t.Fatalf("missing iterator call argument source")
	}
	if arg.Kind != factflow.ValueSourceExpression || !arg.HasExpr {
		t.Fatalf("iterator argument source = %#v, want expression source", arg)
	}
	stateSym := mustLocalAt(t, bindings, stmts[0].(*ast.LocalAssignStmt), 0)
	want := path.NewPath(stateSym, "state").Field("active_sessions")
	semanticFact, ok := result.Call(points[0])
	if !ok || len(semanticFact.ArgumentSources) != 1 {
		t.Fatalf("semantic iterator call fact = %#v/%v", semanticFact, ok)
	}
	if got, ok := pathexpr.Resolve(semanticFact.ArgumentSources[0].Expr, bindings); !ok || !got.Equal(want) {
		t.Fatalf("semantic iterator argument path = %v/%v, want %v", got, ok, want)
	}
	got, ok := facts.ExpressionPath(arg.ExprRef)
	if !ok {
		t.Fatalf("missing expression path for iterator argument ref %d", arg.ExprRef)
	}
	if !got.Equal(want) {
		t.Fatalf("iterator argument path = %v, want %v", got, want)
	}
}

func TestLowerNestedExpressionProducerCallIsReadableSlotZero(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, stmt, 2)
	innerSite, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing inner call site")
	}
	if innerSite.Context() != factflow.CallSiteContextExpressionProducer {
		t.Fatalf("inner call context = %v, want expression producer", innerSite.Context())
	}
	targets := innerSite.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].ResultIndex() != 0 {
		t.Fatalf("inner call targets = %#v", targets)
	}
	producer, ok := callproducer.FromFacts(facts, points[0])
	if !ok {
		t.Fatalf("missing nested call producer")
	}
	if producerTargets := producer.ResultTargets(); len(producerTargets) != 1 || producerTargets[0].Kind() != factflow.CallResultTargetExpression || producerTargets[0].ResultIndex() != 0 {
		t.Fatalf("nested producer targets = %#v", producerTargets)
	}
	outerSite, ok := facts.CallSite(points[1])
	if !ok {
		t.Fatalf("missing outer call site")
	}
	args := outerSite.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceCall || args[0].CallPoint != points[0] || !args[0].HasCallPoint || args[0].ResultIndex != 0 {
		t.Fatalf("outer argument sources = %#v, want inner call source", args)
	}
	if _, ok := callproducer.FromFacts(facts, points[1]); ok {
		t.Fatalf("outer statement call unexpectedly lowered as producer")
	}
}

func TestLowerNestedExpressionProducerCallFromWIRIsReadableSlotZero(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	body := wirlower.Lower("nested-call-producer", stmts, bindings, built)
	facts := Lower(result, built.Graph, Config{Registry: standard.Registry(), Bindings: bindings, WIR: body})
	points := requireStmtPoints(t, built, stmt, 2)
	innerSite, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing inner call site")
	}
	if innerSite.Context() != factflow.CallSiteContextExpressionProducer {
		t.Fatalf("inner WIR call context = %v, want expression producer", innerSite.Context())
	}
	targets := innerSite.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetExpression || targets[0].ResultIndex() != 0 {
		t.Fatalf("inner WIR call targets = %#v", targets)
	}
	producer, ok := callproducer.FromFacts(facts, points[0])
	if !ok {
		t.Fatalf("missing WIR nested call producer")
	}
	if producerTargets := producer.ResultTargets(); len(producerTargets) != 1 || producerTargets[0].Kind() != factflow.CallResultTargetExpression || producerTargets[0].ResultIndex() != 0 {
		t.Fatalf("WIR nested producer targets = %#v", producerTargets)
	}
	outerSite, ok := facts.CallSite(points[1])
	if !ok {
		t.Fatalf("missing outer call site")
	}
	args := outerSite.ArgumentSources()
	if len(args) != 1 || args[0].Kind != factflow.ValueSourceCall || args[0].CallPoint != points[0] || !args[0].HasCallPoint || args[0].ResultIndex != 0 {
		t.Fatalf("outer WIR argument sources = %#v, want inner call source", args)
	}
	if _, ok := callproducer.FromFacts(facts, points[1]); ok {
		t.Fatalf("outer WIR statement call unexpectedly lowered as producer")
	}
}

func TestLowerCallPointForExprCanComeOnlyFromWIRExpressionID(t *testing.T) {
	inner := &ast.FuncCallExpr{Func: ident("g")}
	outer := &ast.FuncCallExpr{Func: ident("f"), Args: []ast.Expr{inner}}
	stmt := &ast.FuncCallStmt{Expr: outer}
	stmts := []ast.Stmt{stmt}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f", "g"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	body := wirlower.Lower("wir-call-points", stmts, bindings, built)
	points := requireStmtPoints(t, built, stmt, 2)

	l := lowerer{
		bindings:             bindings,
		graph:                built.Graph,
		graphID:              built.Graph.ID(),
		wir:                  body,
		wirCallPoints:        callPointsByExpressionIDFromWIR(built.Graph, body),
		exprs:                make(map[any]factflow.ExprRef),
		expressionPaths:      make(map[factflow.ExprRef]path.Path),
		expressionConditions: make(map[factflow.ExprRef]factflow.ExpressionCondition),
	}
	source, ok := l.expressionOperandSource(inner)
	if !ok {
		t.Fatalf("expressionOperandSource(inner) returned false")
	}
	if source.Kind != factflow.ValueSourceCall || !source.HasCallPoint || source.CallPoint != points[0] {
		t.Fatalf("inner call source = %#v, want WIR call point %d", source, points[0])
	}
}

func TestLowerCallPointForExprIgnoresSemanticMapInWIRMode(t *testing.T) {
	call := &ast.FuncCallExpr{Func: ident("g")}
	l := lowerer{
		wir:        wir.NewBody("empty"),
		callPoints: map[*ast.FuncCallExpr]cfg.Point{call: cfg.Point(99)},
	}
	if point, ok := l.callPointForExpr(0, call); ok || point != 0 {
		t.Fatalf("WIR callPointForExpr fell back to semantic map: %d/%v", point, ok)
	}
}

func TestLowerCallSiteMapsUnknownContextExplicitly(t *testing.T) {
	l := lowerer{exprs: make(map[any]factflow.ExprRef)}
	site := l.callSite(semantics.CallFact{
		Context: semantics.CallContextUnknown,
		Call:    &ast.FuncCallExpr{Func: ident("mystery")},
	})
	if site.Context() != factflow.CallSiteContextUnknown {
		t.Fatalf("unknown semantic call context lowered as %v", site.Context())
	}
	if expr, ok := site.Expr(); !ok || expr == 0 {
		t.Fatalf("unknown call-site expr = %d/%v, want explicit expr ref", expr, ok)
	}
}

func TestLowerMemberOrdinaryCallTargetStaysCallSiteOnly(t *testing.T) {
	decl := localAssign([]string{"t"}, &ast.TableExpr{})
	targetRoot := ident("t")
	call := &ast.FuncCallExpr{Func: ident("f")}
	write := assign([]ast.Expr{dot(targetRoot, "x")}, call)
	stmts := []ast.Stmt{decl, write}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"f"}})
	built := cfgbuild.BuildChunk(stmts, bindings)
	result, err := semantics.ExtractChunk(stmts, bindings, built)
	if err != nil {
		t.Fatalf("ExtractChunk: %v", err)
	}

	facts := lowerFacts(t, result, built.Graph, standard.Registry())
	points := requireStmtPoints(t, built, write, 2)
	producer, ok := callproducer.FromFacts(facts, points[0])
	if !ok {
		t.Fatalf("missing assignment call producer at point %d", points[0])
	}
	if producer.CalleeSymbol() == 0 {
		t.Fatalf("producer callee symbol missing")
	}
	if got := producer.ResultTargets(); len(got) != 0 {
		t.Fatalf("member ordinary target leaked into producer targets: %#v", got)
	}

	site, ok := facts.CallSite(points[0])
	if !ok {
		t.Fatalf("missing call site at point %d", points[0])
	}
	if site.Context() != factflow.CallSiteContextAssignmentSource {
		t.Fatalf("call-site context = %v, want assignment source", site.Context())
	}
	targets := site.ResultTargets()
	if len(targets) != 1 || targets[0].Kind() != factflow.CallResultTargetOrdinaryAssignment || targets[0].Index() != 0 || targets[0].ResultIndex() != 0 {
		t.Fatalf("call-site targets = %#v", targets)
	}
	wantPath := path.NewPath(mustIdentSymbol(t, bindings, targetRoot), "t").Field("x")
	if !targets[0].TargetPath().Equal(wantPath) {
		t.Fatalf("call-site target path = %v, want %v", targets[0].TargetPath(), wantPath)
	}
}
