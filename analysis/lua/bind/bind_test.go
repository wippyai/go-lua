package bind

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/compiler/ast"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func number(value string) *ast.NumberExpr {
	return &ast.NumberExpr{Value: value}
}

func localAssign(names []string, exprs ...ast.Expr) *ast.LocalAssignStmt {
	return &ast.LocalAssignStmt{Names: names, Exprs: exprs}
}

func function(names []string, stmts ...ast.Stmt) *ast.FunctionExpr {
	return &ast.FunctionExpr{
		ParList: &ast.ParList{Names: names},
		Stmts:   stmts,
	}
}

func varargFunction(names []string, stmts ...ast.Stmt) *ast.FunctionExpr {
	return &ast.FunctionExpr{
		ParList: &ast.ParList{Names: names, HasVargs: true},
		Stmts:   stmts,
	}
}

func typeRef(name string) *ast.TypeRefExpr {
	return &ast.TypeRefExpr{Path: []string{name}}
}

func ret(exprs ...ast.Expr) *ast.ReturnStmt {
	return &ast.ReturnStmt{Exprs: exprs}
}

func mustSymbol(t *testing.T, r *Result, ident *ast.IdentExpr) symbol.ID {
	t.Helper()
	id, ok := r.SymbolOf(ident)
	if !ok {
		t.Fatalf("SymbolOf(%q) missing", ident.Value)
	}
	return id
}

func mustLocalAt(t *testing.T, r *Result, stmt *ast.LocalAssignStmt, index int) symbol.ID {
	t.Helper()
	id, ok := r.LocalSymbolAt(stmt, index)
	if !ok {
		t.Fatalf("LocalSymbolAt(%d) missing", index)
	}
	return id
}

func mustTypeRef(t *testing.T, r *Result, ref *ast.TypeRefExpr) TypeDecl {
	t.Helper()
	decl, ok := r.TypeRef(ref)
	if !ok {
		t.Fatalf("TypeRef(%v) missing", ref.Path)
	}
	return decl
}

func mustPrimitiveTypeRef(t *testing.T, r *Result, expr *ast.PrimitiveTypeExpr) TypeDecl {
	t.Helper()
	decl, ok := r.PrimitiveTypeRef(expr)
	if !ok {
		t.Fatalf("PrimitiveTypeRef(%q) missing", expr.Name)
	}
	return decl
}

func assertKind(t *testing.T, r *Result, id symbol.ID, want symbol.Kind) {
	t.Helper()
	got, ok := r.Kind(id)
	if !ok {
		t.Fatalf("Kind(%d) missing", id)
	}
	if got != want {
		t.Fatalf("Kind(%d) = %v, want %v", id, got, want)
	}
}

func assertDeclaringFunction(t *testing.T, r *Result, id symbol.ID, want *ast.FunctionExpr) {
	t.Helper()
	got, ok := r.DeclaringFunction(id)
	if !ok {
		t.Fatalf("DeclaringFunction(%d) missing", id)
	}
	if got != want {
		t.Fatalf("DeclaringFunction(%d) = %p, want %p", id, got, want)
	}
}

func mustFunctionSymbol(t *testing.T, r *Result, fn *ast.FunctionExpr) symbol.ID {
	t.Helper()
	id, ok := r.FunctionSymbol(fn)
	if !ok {
		t.Fatalf("FunctionSymbol(%p) missing", fn)
	}
	return id
}

func mustOrigin(t *testing.T, r *Result, fn *ast.FunctionExpr) FunctionOrigin {
	t.Helper()
	origin, ok := r.FunctionOrigin(fn)
	if !ok {
		t.Fatalf("FunctionOrigin(%p) missing", fn)
	}
	return origin
}

func captureIDs(captures []Capture) []symbol.ID {
	ids := make([]symbol.ID, len(captures))
	for i, capture := range captures {
		ids[i] = capture.Captured
	}
	return ids
}

func TestShadowingAndDeferredLocalRules(t *testing.T) {
	outer := localAssign([]string{"x"}, number("1"))
	inner := localAssign([]string{"x"}, number("2"))
	innerRead := ident("x")
	postBlockRead := ident("x")

	r := BindChunk([]ast.Stmt{
		outer,
		&ast.DoBlockStmt{Stmts: []ast.Stmt{
			inner,
			ret(innerRead),
		}},
		ret(postBlockRead),
	}, Options{})

	outerID := mustLocalAt(t, r, outer, 0)
	innerID := mustLocalAt(t, r, inner, 0)
	if got := mustSymbol(t, r, innerRead); got != innerID {
		t.Fatalf("inner read resolved to %d, want inner local %d", got, innerID)
	}
	if got := mustSymbol(t, r, postBlockRead); got != outerID {
		t.Fatalf("post-block read resolved to %d, want outer local %d", got, outerID)
	}

	rhsRead := ident("x")
	shadow := localAssign([]string{"x"}, rhsRead)
	afterShadowRead := ident("x")
	selfRead := ident("f")
	localFn := localAssign([]string{"f"}, function(nil, ret(selfRead)))

	r = BindChunk([]ast.Stmt{
		outer,
		shadow,
		ret(afterShadowRead),
		localFn,
	}, Options{})

	outerID = mustLocalAt(t, r, outer, 0)
	shadowID := mustLocalAt(t, r, shadow, 0)
	fnID := mustLocalAt(t, r, localFn, 0)
	if got := mustSymbol(t, r, rhsRead); got != outerID {
		t.Fatalf("local x = x RHS resolved to %d, want outer local %d", got, outerID)
	}
	if got := mustSymbol(t, r, afterShadowRead); got != shadowID {
		t.Fatalf("post-shadow read resolved to %d, want new local %d", got, shadowID)
	}
	if got := mustSymbol(t, r, selfRead); got != fnID {
		t.Fatalf("initializer closure resolved to %d, want deferred local %d", got, fnID)
	}
}

func TestRepeatUntilConditionSeesBodyLocal(t *testing.T) {
	bodyLocal := localAssign([]string{"again"}, number("1"))
	conditionRead := ident("again")

	r := BindChunk([]ast.Stmt{
		&ast.RepeatStmt{
			Stmts:     []ast.Stmt{bodyLocal},
			Condition: conditionRead,
		},
	}, Options{})

	bodyID := mustLocalAt(t, r, bodyLocal, 0)
	if got := mustSymbol(t, r, conditionRead); got != bodyID {
		t.Fatalf("repeat condition resolved to %d, want body local %d", got, bodyID)
	}
}

func TestImplicitGlobals(t *testing.T) {
	unresolvedRead := ident("missing")
	assignmentTarget := ident("assigned")
	predeclaredRead := ident("print")
	normalizedRead := ident("math")

	r := BindChunk([]ast.Stmt{
		ret(unresolvedRead),
		&ast.AssignStmt{Lhs: []ast.Expr{assignmentTarget}, Rhs: []ast.Expr{number("1")}},
		ret(predeclaredRead, normalizedRead),
	}, Options{Globals: []string{"print", "", "math", "print"}})

	readID := mustSymbol(t, r, unresolvedRead)
	assertKind(t, r, readID, symbol.Global)
	if !r.IsImplicitGlobalUse(unresolvedRead) {
		t.Fatalf("unresolved read was not marked implicit global use")
	}

	assignID := mustSymbol(t, r, assignmentTarget)
	assertKind(t, r, assignID, symbol.Global)
	if r.IsImplicitGlobalUse(assignmentTarget) {
		t.Fatalf("assignment target was marked implicit global read")
	}

	for _, read := range []*ast.IdentExpr{predeclaredRead, normalizedRead} {
		id := mustSymbol(t, r, read)
		assertKind(t, r, id, symbol.Global)
		if r.IsImplicitGlobalUse(read) {
			t.Fatalf("predeclared global %q was marked implicit", read.Value)
		}
	}
	if !r.ResolvesToGlobal(predeclaredRead, "print") {
		t.Fatalf("predeclared read did not resolve to global print")
	}
	if !r.ResolvesToGlobal(unresolvedRead, "missing") {
		t.Fatalf("implicit read did not resolve to global missing")
	}
	if r.ResolvesToGlobal(predeclaredRead, "math") {
		t.Fatalf("print read resolved to wrong global")
	}
	if r.ResolvesToGlobal(nil, "print") {
		t.Fatalf("nil ident resolved to global")
	}

	gotNames := PredeclaredGlobalNames(map[string]int{"": 1, "b": 2, "a": 3})
	if want := []string{"a", "b"}; !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("PredeclaredGlobalNames = %v, want %v", gotNames, want)
	}
}

func TestFuncDefTargetSymbol(t *testing.T) {
	globalTarget := ident("f")
	globalStmt := &ast.FuncDefStmt{Name: &ast.FuncName{Func: globalTarget}, Func: function(nil)}
	localDecl := localAssign([]string{"f"})
	localTarget := ident("f")
	localStmt := &ast.FuncDefStmt{Name: &ast.FuncName{Func: localTarget}, Func: function(nil)}
	dottedStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Func: &ast.AttrGetExpr{
			Object:    ident("module"),
			Key:       &ast.StringExpr{Value: "f"},
			KeySyntax: ast.AttrKeyDot,
		}},
		Func: function(nil),
	}
	methodStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Receiver: ident("module"), Method: "f"},
		Func: function(nil),
	}

	r := BindChunk([]ast.Stmt{globalStmt, localDecl, localStmt, dottedStmt, methodStmt}, Options{})

	globalID := mustSymbol(t, r, globalTarget)
	id, ok := r.FuncDefTargetSymbol(globalStmt)
	if !ok || id != globalID {
		t.Fatalf("global FuncDefTargetSymbol = %d/%v, want %d/true", id, ok, globalID)
	}
	assertKind(t, r, id, symbol.Global)

	localID := mustLocalAt(t, r, localDecl, 0)
	id, ok = r.FuncDefTargetSymbol(localStmt)
	if !ok || id != localID {
		t.Fatalf("local FuncDefTargetSymbol = %d/%v, want %d/true", id, ok, localID)
	}

	for _, tt := range []struct {
		name string
		stmt *ast.FuncDefStmt
	}{
		{name: "dotted", stmt: dottedStmt},
		{name: "method", stmt: methodStmt},
		{name: "nil stmt", stmt: nil},
		{name: "nil name", stmt: &ast.FuncDefStmt{Func: function(nil)}},
		{name: "nil func name", stmt: &ast.FuncDefStmt{Name: &ast.FuncName{}, Func: function(nil)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if id, ok := r.FuncDefTargetSymbol(tt.stmt); ok || id != 0 {
				t.Fatalf("FuncDefTargetSymbol = %d/%v, want 0/false", id, ok)
			}
		})
	}

	var nilResult *Result
	if id, ok := nilResult.FuncDefTargetSymbol(globalStmt); ok || id != 0 {
		t.Fatalf("nil result FuncDefTargetSymbol = %d/%v, want 0/false", id, ok)
	}
}

func TestParamSymbols(t *testing.T) {
	aRead := ident("a")
	bRead := ident("b")
	fn := function([]string{"a", "b"}, ret(aRead, bRead))
	r := BindFunction(fn, Options{})

	params := r.ParamSymbols(fn)
	if len(params) != 2 {
		t.Fatalf("got %d params, want 2", len(params))
	}
	for i, wantName := range []string{"a", "b"} {
		if got := r.Name(params[i]); got != wantName {
			t.Fatalf("param %d name = %q, want %q", i, got, wantName)
		}
		assertKind(t, r, params[i], symbol.Param)
	}
	if got := mustSymbol(t, r, aRead); got != params[0] {
		t.Fatalf("a read resolved to %d, want param %d", got, params[0])
	}
	if got := mustSymbol(t, r, bRead); got != params[1] {
		t.Fatalf("b read resolved to %d, want param %d", got, params[1])
	}

	varargFn := &ast.FunctionExpr{ParList: &ast.ParList{Names: []string{"x"}, HasVargs: true}}
	r = BindFunction(varargFn, Options{})
	if params := r.ParamSymbols(varargFn); len(params) != 1 || r.Name(params[0]) != "x" {
		t.Fatalf("vararg function params = %v, want only x", params)
	}

	typedFn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"typed"},
			Types: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
		},
	}
	r = BindFunction(typedFn, Options{})
	params = r.ParamSymbols(typedFn)
	if len(params) != 1 || r.Name(params[0]) != "typed" {
		t.Fatalf("typed params = %v, want typed param", params)
	}
	assertKind(t, r, params[0], symbol.Param)
}

func TestFunctionIdentityAndTree(t *testing.T) {
	grandchild := function(nil)
	childA := function(nil, localAssign([]string{"grandchild"}, grandchild))
	childB := function(nil)
	root := function(nil,
		localAssign([]string{"childA"}, childA),
		localAssign([]string{"childB"}, childB),
	)

	r := BindFunction(root, Options{})

	wantFunctions := []*ast.FunctionExpr{root, childA, grandchild, childB}
	for _, fn := range wantFunctions {
		first, ok := r.FunctionSymbol(fn)
		if !ok || first == 0 {
			t.Fatalf("FunctionSymbol(%p) = %d/%v, want non-zero/true", fn, first, ok)
		}
		second, ok := r.FunctionSymbol(fn)
		if !ok || second != first {
			t.Fatalf("second FunctionSymbol(%p) = %d/%v, want stable %d/true", fn, second, ok, first)
		}
		assertKind(t, r, first, symbol.Function)

		gotFn, ok := r.FunctionBySymbol(first)
		if !ok || gotFn != fn {
			t.Fatalf("FunctionBySymbol(%d) = %p/%v, want %p/true", first, gotFn, ok, fn)
		}
		assertDeclaringFunction(t, r, first, fn)
	}

	if got := r.Functions(); !reflect.DeepEqual(got, wantFunctions) {
		t.Fatalf("Functions() = %p, want parent-before-child %p", got, wantFunctions)
	}
	functions := r.Functions()
	functions[0] = nil
	if got := r.Functions(); got[0] != root {
		t.Fatalf("Functions returned mutable backing slice")
	}

	if got, want := r.NestedFunctions(nil), []*ast.FunctionExpr{root}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NestedFunctions(nil) = %p, want %p", got, want)
	}
	if got, want := r.NestedFunctions(root), []*ast.FunctionExpr{childA, childB}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NestedFunctions(root) = %p, want %p", got, want)
	}
	if got, want := r.NestedFunctions(childA), []*ast.FunctionExpr{grandchild}; !reflect.DeepEqual(got, want) {
		t.Fatalf("NestedFunctions(childA) = %p, want %p", got, want)
	}
	if got := r.NestedFunctions(childB); got != nil {
		t.Fatalf("NestedFunctions(childB) = %p, want nil", got)
	}

	unknown := function(nil)
	if id, ok := r.FunctionSymbol(unknown); ok || id != 0 {
		t.Fatalf("unknown FunctionSymbol = %d/%v, want 0/false", id, ok)
	}
	if fn, ok := r.FunctionBySymbol(symbol.ID(0)); ok || fn != nil {
		t.Fatalf("zero FunctionBySymbol = %p/%v, want nil/false", fn, ok)
	}

	rootOrigin := mustOrigin(t, r, root)
	if rootOrigin.Kind != FunctionOriginLiteral || rootOrigin.Parent != nil || rootOrigin.Symbol != mustFunctionSymbol(t, r, root) {
		t.Fatalf("root origin = %#v, want literal root with function symbol", rootOrigin)
	}
	childParent, ok := r.ParentFunction(childA)
	if !ok || childParent != root {
		t.Fatalf("ParentFunction(childA) = %p/%v, want root/true", childParent, ok)
	}
	rootParent, ok := r.ParentFunction(root)
	if !ok || rootParent != nil {
		t.Fatalf("ParentFunction(root) = %p/%v, want nil/true", rootParent, ok)
	}
	if parent, ok := r.ParentFunction(unknown); ok || parent != nil {
		t.Fatalf("unknown ParentFunction = %p/%v, want nil/false", parent, ok)
	}
}

func TestFunctionOrigins(t *testing.T) {
	nestedFn := function(nil)
	nestedStmt := localAssign([]string{"nested"}, nestedFn)
	globalTarget := ident("globalFn")
	globalFn := function(nil, nestedStmt)
	globalStmt := &ast.FuncDefStmt{Name: &ast.FuncName{Func: globalTarget}, Func: globalFn}

	localFunctionFn := function(nil)
	localFunctionStmt := localAssign([]string{"localFunction"}, localFunctionFn)

	localAssignmentFn := function(nil)
	localAssignmentStmt := localAssign([]string{"value", "localAssignment"}, number("1"), localAssignmentFn)

	methodFn := function(nil)
	methodStmt := &ast.FuncDefStmt{
		Name: &ast.FuncName{Receiver: ident("obj"), Method: "method"},
		Func: methodFn,
	}

	literalFn := function(nil)
	literalStmt := localAssign([]string{"table"}, &ast.TableExpr{Fields: []*ast.Field{{
		Value: literalFn,
	}}})

	stmts := []ast.Stmt{globalStmt, localFunctionStmt, localAssignmentStmt, methodStmt, literalStmt}
	r := BindChunk(stmts, Options{Globals: []string{"obj"}})

	wantOrder := []*ast.FunctionExpr{globalFn, nestedFn, localFunctionFn, localAssignmentFn, methodFn, literalFn}
	if got := r.Functions(); !reflect.DeepEqual(got, wantOrder) {
		t.Fatalf("Functions() = %p, want %p", got, wantOrder)
	}
	origins := r.FunctionOrigins()
	if len(origins) != len(wantOrder) {
		t.Fatalf("FunctionOrigins length = %d, want %d", len(origins), len(wantOrder))
	}
	for i, fn := range wantOrder {
		if origins[i].Func != fn || origins[i].Symbol != mustFunctionSymbol(t, r, fn) {
			t.Fatalf("origin %d = %#v, want function %p with its symbol", i, origins[i], fn)
		}
	}

	globalOrigin := mustOrigin(t, r, globalFn)
	globalTargetID := mustSymbol(t, r, globalTarget)
	if globalOrigin.Kind != FunctionOriginDeclaration || globalOrigin.Parent != nil || globalOrigin.Stmt != globalStmt || globalOrigin.LocalIndex != -1 {
		t.Fatalf("global function origin = %#v, want declaration on stmt", globalOrigin)
	}
	if !globalOrigin.HasTargetSymbol || globalOrigin.TargetSymbol != globalTargetID {
		t.Fatalf("global target symbol = %d/%v, want %d/true", globalOrigin.TargetSymbol, globalOrigin.HasTargetSymbol, globalTargetID)
	}

	nestedOrigin := mustOrigin(t, r, nestedFn)
	nestedTargetID := mustLocalAt(t, r, nestedStmt, 0)
	if nestedOrigin.Kind != FunctionOriginLocalAssignment || nestedOrigin.Parent != globalFn || nestedOrigin.Stmt != nestedStmt || nestedOrigin.LocalIndex != 0 {
		t.Fatalf("nested origin = %#v, want local assignment under globalFn", nestedOrigin)
	}
	if !nestedOrigin.HasTargetSymbol || nestedOrigin.TargetSymbol != nestedTargetID {
		t.Fatalf("nested target symbol = %d/%v, want %d/true", nestedOrigin.TargetSymbol, nestedOrigin.HasTargetSymbol, nestedTargetID)
	}

	// Current AST lowers local-function syntax to a local assignment shape, so
	// bind records the local statement/index rather than inventing syntax.
	localFunctionOrigin := mustOrigin(t, r, localFunctionFn)
	localFunctionTargetID := mustLocalAt(t, r, localFunctionStmt, 0)
	if localFunctionOrigin.Kind != FunctionOriginLocalAssignment || localFunctionOrigin.Stmt != localFunctionStmt || localFunctionOrigin.LocalIndex != 0 {
		t.Fatalf("local function origin = %#v, want normalized local assignment", localFunctionOrigin)
	}
	if !localFunctionOrigin.HasTargetSymbol || localFunctionOrigin.TargetSymbol != localFunctionTargetID {
		t.Fatalf("local function target = %d/%v, want %d/true", localFunctionOrigin.TargetSymbol, localFunctionOrigin.HasTargetSymbol, localFunctionTargetID)
	}

	localAssignmentOrigin := mustOrigin(t, r, localAssignmentFn)
	localAssignmentTargetID := mustLocalAt(t, r, localAssignmentStmt, 1)
	if localAssignmentOrigin.Kind != FunctionOriginLocalAssignment || localAssignmentOrigin.Stmt != localAssignmentStmt || localAssignmentOrigin.LocalIndex != 1 {
		t.Fatalf("local assignment origin = %#v, want local assignment index 1", localAssignmentOrigin)
	}
	if !localAssignmentOrigin.HasTargetSymbol || localAssignmentOrigin.TargetSymbol != localAssignmentTargetID {
		t.Fatalf("local assignment target = %d/%v, want %d/true", localAssignmentOrigin.TargetSymbol, localAssignmentOrigin.HasTargetSymbol, localAssignmentTargetID)
	}

	methodOrigin := mustOrigin(t, r, methodFn)
	if methodOrigin.Kind != FunctionOriginMethod || methodOrigin.Stmt != methodStmt || methodOrigin.Method != "method" {
		t.Fatalf("method origin = %#v, want method origin with method name", methodOrigin)
	}
	if methodOrigin.HasTargetSymbol || methodOrigin.TargetSymbol != 0 {
		t.Fatalf("method target = %d/%v, want no target symbol", methodOrigin.TargetSymbol, methodOrigin.HasTargetSymbol)
	}

	literalOrigin := mustOrigin(t, r, literalFn)
	if literalOrigin.Kind != FunctionOriginLiteral || literalOrigin.Parent != nil || literalOrigin.Stmt != nil || literalOrigin.LocalIndex != -1 {
		t.Fatalf("literal origin = %#v, want side-effect-free literal metadata", literalOrigin)
	}

	origins[0].Kind = FunctionOriginUnknown
	if got := mustOrigin(t, r, globalFn).Kind; got != FunctionOriginDeclaration {
		t.Fatalf("FunctionOrigins returned mutable origin storage; kind = %v", got)
	}
}

func TestDeclaringFunctionForLexicalSymbols(t *testing.T) {
	paramRead := ident("p")
	localStmt := localAssign([]string{"x"}, number("1"))
	loopRead := ident("i")
	numLoop := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: number("3"),
		Stmts: []ast.Stmt{ret(loopRead)},
	}
	fn := function([]string{"p"}, localStmt, numLoop, ret(paramRead))
	topLocal := localAssign([]string{"fn"}, fn)

	r := BindChunk([]ast.Stmt{topLocal}, Options{})

	paramID := r.ParamSymbols(fn)[0]
	assertDeclaringFunction(t, r, paramID, fn)

	localID := mustLocalAt(t, r, localStmt, 0)
	assertDeclaringFunction(t, r, localID, fn)

	loopID, ok := r.NumForSymbol(numLoop)
	if !ok {
		t.Fatalf("NumForSymbol missing")
	}
	assertDeclaringFunction(t, r, loopID, fn)

	if got := mustSymbol(t, r, paramRead); got != paramID {
		t.Fatalf("param read resolved to %d, want %d", got, paramID)
	}
	if got := mustSymbol(t, r, loopRead); got != loopID {
		t.Fatalf("loop read resolved to %d, want %d", got, loopID)
	}

	topID := mustLocalAt(t, r, topLocal, 0)
	if got, ok := r.DeclaringFunction(topID); ok || got != nil {
		t.Fatalf("top-level DeclaringFunction = %p/%v, want nil/false", got, ok)
	}
}

func TestDirectCapturesOuterParamLocalAndGlobals(t *testing.T) {
	localWrite := ident("x")
	paramRead := ident("p")
	localRead := ident("x")
	globalRead := ident("print")
	child := function(nil,
		&ast.AssignStmt{Lhs: []ast.Expr{localWrite}, Rhs: []ast.Expr{number("2")}},
		ret(paramRead, localRead, globalRead),
	)
	outerLocal := localAssign([]string{"x"}, number("1"))
	outer := function([]string{"p"},
		outerLocal,
		localAssign([]string{"child"}, child),
	)

	r := BindFunction(outer, Options{Globals: []string{"print"}})

	localID := mustLocalAt(t, r, outerLocal, 0)
	paramID := r.ParamSymbols(outer)[0]
	if got := mustSymbol(t, r, localWrite); got != localID {
		t.Fatalf("outer local write resolved to %d, want %d", got, localID)
	}
	if got := mustSymbol(t, r, localRead); got != localID {
		t.Fatalf("outer local read resolved to %d, want %d", got, localID)
	}
	if got := mustSymbol(t, r, paramRead); got != paramID {
		t.Fatalf("outer param read resolved to %d, want %d", got, paramID)
	}
	assertKind(t, r, localID, symbol.Local)
	assertKind(t, r, paramID, symbol.Param)
	assertKind(t, r, mustSymbol(t, r, globalRead), symbol.Global)

	captures := r.DirectCaptures(child)
	if got, want := captureIDs(captures), []symbol.ID{localID, paramID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectCaptures(child) = %v, want %v", got, want)
	}
	for i, want := range []struct {
		id   symbol.ID
		name string
	}{
		{id: localID, name: "x"},
		{id: paramID, name: "p"},
	} {
		if captures[i].Captured != want.id || captures[i].CapturedName != want.name || captures[i].DeclaringFunction != outer {
			t.Fatalf("capture %d = %#v, want %q declared by outer", i, captures[i], want.name)
		}
	}
	if got := r.DirectCaptures(outer); got != nil {
		t.Fatalf("DirectCaptures(outer) = %#v, want nil", got)
	}

	captures[0].CapturedName = "mutated"
	if got := r.DirectCaptures(child)[0].CapturedName; got != "x" {
		t.Fatalf("DirectCaptures returned mutable backing slice; name = %q", got)
	}
}

func TestDirectCapturesNestedNonVarargDoesNotCaptureOuterVararg(t *testing.T) {
	localRead := ident("x")
	child := function(nil, ret(&ast.Comma3Expr{}, localRead))
	outerLocal := localAssign([]string{"x"}, number("1"))
	outer := varargFunction(nil,
		outerLocal,
		localAssign([]string{"child"}, child),
	)

	r := BindFunction(outer, Options{})

	localID := mustLocalAt(t, r, outerLocal, 0)
	varargID, ok := r.VarargSymbol(outer)
	if !ok {
		t.Fatalf("outer VarargSymbol missing")
	}
	if got := mustSymbol(t, r, localRead); got != localID {
		t.Fatalf("outer local read resolved to %d, want %d", got, localID)
	}

	captures := r.DirectCaptures(child)
	if got, want := captureIDs(captures), []symbol.ID{localID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectCaptures(child) = %v, want %v without outer vararg %d", got, want, varargID)
	}
	if captures[0].CapturedName != "x" || captures[0].DeclaringFunction != outer {
		t.Fatalf("capture = %#v, want x declared by outer", captures[0])
	}
}

func TestDirectCapturesShadowingAndNestedDirectness(t *testing.T) {
	outerX := localAssign([]string{"x"}, number("1"))
	outerOnly := localAssign([]string{"outerOnly"}, number("2"))
	shadowX := localAssign([]string{"x"}, number("3"))
	shadowRead := ident("x")
	shadowUse := localAssign([]string{"shadowUse"}, shadowRead)
	parentOnly := localAssign([]string{"parentOnly"}, number("4"))
	parentOnlyRead := ident("parentOnly")
	outerOnlyRead := ident("outerOnly")
	grandchild := function(nil, ret(parentOnlyRead, outerOnlyRead))
	child := function(nil,
		shadowX,
		shadowUse,
		parentOnly,
		localAssign([]string{"grandchild"}, grandchild),
	)
	outer := function(nil,
		outerX,
		outerOnly,
		localAssign([]string{"child"}, child),
	)

	r := BindFunction(outer, Options{})

	shadowID := mustLocalAt(t, r, shadowX, 0)
	if got := mustSymbol(t, r, shadowRead); got != shadowID {
		t.Fatalf("shadowed x read resolved to %d, want child local %d", got, shadowID)
	}
	if got := r.DirectCaptures(child); got != nil {
		t.Fatalf("DirectCaptures(child) = %#v, want nil for shadowed and grandchild-only uses", got)
	}

	parentOnlyID := mustLocalAt(t, r, parentOnly, 0)
	outerOnlyID := mustLocalAt(t, r, outerOnly, 0)
	captures := r.DirectCaptures(grandchild)
	if got, want := captureIDs(captures), []symbol.ID{parentOnlyID, outerOnlyID}; !reflect.DeepEqual(got, want) {
		t.Fatalf("DirectCaptures(grandchild) = %v, want %v", got, want)
	}
	if captures[0].CapturedName != "parentOnly" || captures[0].DeclaringFunction != child {
		t.Fatalf("grandchild parent capture = %#v, want parentOnly declared by child", captures[0])
	}
	if captures[1].CapturedName != "outerOnly" || captures[1].DeclaringFunction != outer {
		t.Fatalf("grandchild outer capture = %#v, want outerOnly declared by outer", captures[1])
	}
}

func TestParamSlotsTypedAndVararg(t *testing.T) {
	xType := typeRef("X")
	yType := typeRef("Y")
	varargType := typeRef("Rest")
	fn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names:      []string{"x", "y"},
			Types:      []ast.TypeExpr{xType, yType},
			HasVargs:   true,
			VarargType: varargType,
		},
	}

	r := BindFunction(fn, Options{})

	params := r.ParamSymbols(fn)
	if len(params) != 2 {
		t.Fatalf("ParamSymbols = %v, want only named params", params)
	}
	slots := r.ParamSlots(fn)
	if len(slots) != 3 {
		t.Fatalf("ParamSlots = %#v, want x, y, vararg", slots)
	}
	for i, tt := range []struct {
		name string
		typ  ast.TypeExpr
	}{
		{name: "x", typ: xType},
		{name: "y", typ: yType},
	} {
		if slots[i].Symbol != params[i] || slots[i].Name != tt.name || slots[i].Type != tt.typ {
			t.Fatalf("slot %d = %#v, want symbol %d name %q type %p", i, slots[i], params[i], tt.name, tt.typ)
		}
		if slots[i].SourceIndex != i {
			t.Fatalf("slot %d source index = %d, want %d", i, slots[i].SourceIndex, i)
		}
		if slots[i].Vararg || slots[i].ImplicitSelf {
			t.Fatalf("slot %d flags = vararg %v implicit self %v, want false/false", i, slots[i].Vararg, slots[i].ImplicitSelf)
		}
	}
	varargID, ok := r.VarargSymbol(fn)
	if !ok || varargID == 0 {
		t.Fatalf("VarargSymbol = %d/%v, want non-zero/true", varargID, ok)
	}
	assertKind(t, r, varargID, symbol.Param)
	if slots[2].Symbol != varargID || slots[2].Name != "..." || slots[2].Type != varargType || slots[2].SourceIndex != 2 || !slots[2].Vararg || slots[2].ImplicitSelf {
		t.Fatalf("vararg slot = %#v, want typed vararg symbol %d", slots[2], varargID)
	}
	assertDeclaringFunction(t, r, varargID, fn)

	slots[0].Name = "mutated"
	if got := r.ParamSlots(fn)[0].Name; got != "x" {
		t.Fatalf("ParamSlots returned mutable backing slice; name = %q", got)
	}
}

func TestParamSlotsMethodSelf(t *testing.T) {
	implicitFn := function([]string{"arg"})
	r := BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{Receiver: ident("obj"), Method: "method"},
			Func: implicitFn,
		},
	}, Options{Globals: []string{"obj"}})

	params := r.ParamSymbols(implicitFn)
	slots := r.ParamSlots(implicitFn)
	if len(params) != 2 || len(slots) != 2 {
		t.Fatalf("implicit method params/slots = %v/%#v, want two", params, slots)
	}
	if slots[0].Symbol != params[0] || slots[0].Name != "self" || slots[0].SourceIndex != -1 || !slots[0].ImplicitSelf || slots[0].Vararg {
		t.Fatalf("implicit self slot = %#v, want implicit self", slots[0])
	}
	if slots[1].Symbol != params[1] || slots[1].Name != "arg" || slots[1].SourceIndex != 0 || slots[1].ImplicitSelf || slots[1].Vararg {
		t.Fatalf("implicit method arg slot = %#v, want explicit arg", slots[1])
	}

	selfType := typeRef("Self")
	explicitFn := &ast.FunctionExpr{
		ParList: &ast.ParList{
			Names: []string{"self", "arg"},
			Types: []ast.TypeExpr{selfType},
		},
	}
	r = BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{Receiver: ident("obj"), Method: "method"},
			Func: explicitFn,
		},
	}, Options{Globals: []string{"obj"}})

	params = r.ParamSymbols(explicitFn)
	slots = r.ParamSlots(explicitFn)
	if len(params) != 2 || len(slots) != 2 {
		t.Fatalf("explicit method params/slots = %v/%#v, want two", params, slots)
	}
	if slots[0].Symbol != params[0] || slots[0].Name != "self" || slots[0].Type != selfType || slots[0].SourceIndex != 0 || slots[0].ImplicitSelf {
		t.Fatalf("explicit self slot = %#v, want typed explicit self", slots[0])
	}
	if slots[1].Symbol != params[1] || slots[1].Name != "arg" || slots[1].SourceIndex != 1 || slots[1].ImplicitSelf {
		t.Fatalf("explicit method arg slot = %#v, want explicit arg", slots[1])
	}
}

func TestMethodSelfParam(t *testing.T) {
	receiver := ident("obj")
	selfRead := ident("self")
	argRead := ident("arg")
	methodFn := function([]string{"arg"}, ret(selfRead, argRead))

	r := BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{Receiver: receiver, Method: "method"},
			Func: methodFn,
		},
	}, Options{Globals: []string{"obj"}})

	params := r.ParamSymbols(methodFn)
	if len(params) != 2 {
		t.Fatalf("method params = %v, want self and arg", params)
	}
	if r.Name(params[0]) != "self" || r.Name(params[1]) != "arg" {
		t.Fatalf("method param names = %q, %q; want self, arg", r.Name(params[0]), r.Name(params[1]))
	}
	if got := mustSymbol(t, r, selfRead); got != params[0] {
		t.Fatalf("self read resolved to %d, want implicit self %d", got, params[0])
	}
	if got := mustSymbol(t, r, argRead); got != params[1] {
		t.Fatalf("arg read resolved to %d, want arg param %d", got, params[1])
	}
	if r.IsImplicitGlobalUse(receiver) {
		t.Fatalf("predeclared method receiver was marked implicit")
	}

	explicitSelf := ident("self")
	explicitFn := function([]string{"self", "arg"}, ret(explicitSelf))
	r = BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{Receiver: ident("obj"), Method: "method"},
			Func: explicitFn,
		},
	}, Options{Globals: []string{"obj"}})

	params = r.ParamSymbols(explicitFn)
	if len(params) != 2 {
		t.Fatalf("explicit-self method params = %v, want exactly 2", params)
	}
	if r.Name(params[0]) != "self" || r.Name(params[1]) != "arg" {
		t.Fatalf("explicit-self method param names = %q, %q; want self, arg", r.Name(params[0]), r.Name(params[1]))
	}
	if got := mustSymbol(t, r, explicitSelf); got != params[0] {
		t.Fatalf("explicit self read resolved to %d, want first param %d", got, params[0])
	}
}

func TestFunctionDefinitionsAndNestedScopes(t *testing.T) {
	target := ident("f")
	bodyRead := ident("f")
	globalFn := function(nil, ret(bodyRead))
	r := BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{Name: &ast.FuncName{Func: target}, Func: globalFn},
	}, Options{})

	targetID := mustSymbol(t, r, target)
	if got := mustSymbol(t, r, bodyRead); got != targetID {
		t.Fatalf("global function body read resolved to %d, want function target %d", got, targetID)
	}
	assertKind(t, r, targetID, symbol.Global)
	if r.IsImplicitGlobalUse(target) {
		t.Fatalf("global function assignment target was marked implicit")
	}
	if r.IsImplicitGlobalUse(bodyRead) {
		t.Fatalf("global function body read was marked implicit after function definition created the global")
	}

	receiver := ident("mod")
	r = BindChunk([]ast.Stmt{
		&ast.FuncDefStmt{
			Name: &ast.FuncName{
				Func: &ast.AttrGetExpr{
					Object:    receiver,
					Key:       &ast.StringExpr{Value: "f"},
					KeySyntax: ast.AttrKeyDot,
				},
			},
			Func: function(nil),
		},
	}, Options{})
	if id := mustSymbol(t, r, receiver); r.Name(id) != "mod" {
		t.Fatalf("dotted receiver name = %q, want mod", r.Name(id))
	}
	if !r.IsImplicitGlobalUse(receiver) {
		t.Fatalf("dotted function receiver was not bound as an implicit global read")
	}

	gRead := ident("g")
	fRead := ident("f")
	mutual := localAssign([]string{"f", "g"},
		function(nil, ret(gRead)),
		function(nil, ret(fRead)),
	)
	r = BindChunk([]ast.Stmt{mutual}, Options{})
	fID := mustLocalAt(t, r, mutual, 0)
	gID := mustLocalAt(t, r, mutual, 1)
	if got := mustSymbol(t, r, gRead); got != gID {
		t.Fatalf("f body g read resolved to %d, want g local %d", got, gID)
	}
	if got := mustSymbol(t, r, fRead); got != fID {
		t.Fatalf("g body f read resolved to %d, want f local %d", got, fID)
	}

	outer := localAssign([]string{"x"}, number("1"))
	paramRead := ident("x")
	nested := function([]string{"x"}, ret(paramRead))
	localNested := localAssign([]string{"nested"}, nested)
	outerRead := ident("x")
	r = BindChunk([]ast.Stmt{outer, localNested, ret(outerRead)}, Options{})
	outerID := mustLocalAt(t, r, outer, 0)
	paramID := r.ParamSymbols(nested)[0]
	if got := mustSymbol(t, r, paramRead); got != paramID {
		t.Fatalf("nested x read resolved to %d, want param %d", got, paramID)
	}
	if got := mustSymbol(t, r, outerRead); got != outerID {
		t.Fatalf("outer x read resolved to %d, want outer local %d", got, outerID)
	}
}

func TestLoopLocals(t *testing.T) {
	outer := localAssign([]string{"i"}, number("0"))
	limitRead := ident("i")
	bodyRead := ident("i")
	postRead := ident("i")
	numLoop := &ast.NumberForStmt{
		Name:  "i",
		Init:  number("1"),
		Limit: limitRead,
		Stmts: []ast.Stmt{ret(bodyRead)},
	}

	r := BindChunk([]ast.Stmt{outer, numLoop, ret(postRead)}, Options{})
	outerID := mustLocalAt(t, r, outer, 0)
	loopID, ok := r.NumForSymbol(numLoop)
	if !ok {
		t.Fatalf("NumForSymbol missing")
	}
	if r.Name(loopID) != "i" {
		t.Fatalf("numeric for symbol name = %q, want i", r.Name(loopID))
	}
	assertKind(t, r, loopID, symbol.Local)
	if got := mustSymbol(t, r, limitRead); got != outerID {
		t.Fatalf("numeric for limit resolved to %d, want outer local %d", got, outerID)
	}
	if got := mustSymbol(t, r, bodyRead); got != loopID {
		t.Fatalf("numeric for body resolved to %d, want loop local %d", got, loopID)
	}
	if got := mustSymbol(t, r, postRead); got != outerID {
		t.Fatalf("post-loop read resolved to %d, want outer local %d", got, outerID)
	}

	iterRead := ident("iter")
	kRead := ident("k")
	vRead := ident("v")
	genLoop := &ast.GenericForStmt{
		Names: []string{"k", "v"},
		Exprs: []ast.Expr{iterRead},
		Stmts: []ast.Stmt{ret(kRead, vRead)},
	}
	r = BindChunk([]ast.Stmt{genLoop}, Options{Globals: []string{"iter"}})
	ids := r.GenericForSymbols(genLoop)
	if len(ids) != 2 {
		t.Fatalf("generic for symbols = %v, want 2", ids)
	}
	if got := mustSymbol(t, r, kRead); got != ids[0] {
		t.Fatalf("generic k read resolved to %d, want %d", got, ids[0])
	}
	if got := mustSymbol(t, r, vRead); got != ids[1] {
		t.Fatalf("generic v read resolved to %d, want %d", got, ids[1])
	}
	if r.IsImplicitGlobalUse(iterRead) {
		t.Fatalf("predeclared iterator was marked implicit")
	}
}

func TestTypedNamesDoNotAffectValueSymbols(t *testing.T) {
	stmt := &ast.LocalAssignStmt{
		Names: []string{"a", "b"},
		Types: []ast.TypeExpr{&ast.TypeRefExpr{Path: []string{"T"}}},
		Exprs: []ast.Expr{number("1"), number("2")},
	}

	r := BindChunk([]ast.Stmt{
		&ast.TypeDefStmt{Name: "Alias", Type: &ast.TypeRefExpr{Path: []string{"Missing"}}},
		&ast.InterfaceDefStmt{Name: "Iface", Extends: []*ast.TypeRefExpr{{Path: []string{"Base"}}}},
		stmt,
	}, Options{})

	ids := r.LocalSymbols(stmt)
	if len(ids) != 2 {
		t.Fatalf("local symbols = %v, want 2 despite one type annotation", ids)
	}
	if len(r.names) != 2 {
		t.Fatalf("allocated %d symbols, want only the two local value symbols", len(r.names))
	}
	if r.Name(ids[0]) != "a" || r.Name(ids[1]) != "b" {
		t.Fatalf("local symbol names = %q, %q; want a, b", r.Name(ids[0]), r.Name(ids[1]))
	}
}

func TestLexicalTypeNames(t *testing.T) {
	outerDef := &ast.TypeDefStmt{Name: "Value", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	outerUse := typeRef("Value")
	outerPrimitiveUse := &ast.PrimitiveTypeExpr{Name: "Value"}
	innerDef := &ast.TypeDefStmt{Name: "Value", Type: &ast.PrimitiveTypeExpr{Name: "string"}}
	innerUse := typeRef("Value")
	afterUse := typeRef("Value")
	blockDef := &ast.TypeDefStmt{Name: "LocalPoint", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	outsideBlockUse := typeRef("LocalPoint")
	beforeDefUse := typeRef("Point")
	laterDef := &ast.TypeDefStmt{Name: "Point", Type: &ast.PrimitiveTypeExpr{Name: "number"}}
	selfUse := typeRef("Node")
	selfDef := &ast.TypeDefStmt{Name: "Node", Type: &ast.RecordTypeExpr{Fields: []ast.RecordFieldExpr{{
		Name: "next",
		Type: selfUse,
	}}}}

	r := BindChunk([]ast.Stmt{
		outerDef,
		&ast.LocalAssignStmt{Names: []string{"a"}, Types: []ast.TypeExpr{outerUse}},
		&ast.LocalAssignStmt{Names: []string{"aa"}, Types: []ast.TypeExpr{outerPrimitiveUse}},
		&ast.IfStmt{Condition: &ast.TrueExpr{}, Then: []ast.Stmt{
			innerDef,
			&ast.LocalAssignStmt{Names: []string{"b"}, Types: []ast.TypeExpr{innerUse}},
		}},
		&ast.LocalAssignStmt{Names: []string{"c"}, Types: []ast.TypeExpr{afterUse}},
		&ast.DoBlockStmt{Stmts: []ast.Stmt{blockDef}},
		&ast.LocalAssignStmt{Names: []string{"outside"}, Types: []ast.TypeExpr{outsideBlockUse}},
		&ast.LocalAssignStmt{Names: []string{"before"}, Types: []ast.TypeExpr{beforeDefUse}},
		laterDef,
		selfDef,
	}, Options{})

	outerDecl, ok := r.TypeDef(outerDef)
	if !ok {
		t.Fatalf("outer type declaration missing")
	}
	innerDecl, ok := r.TypeDef(innerDef)
	if !ok {
		t.Fatalf("inner type declaration missing")
	}
	if got := mustTypeRef(t, r, outerUse); got.ID != outerDecl.ID {
		t.Fatalf("outer use resolved to %#v, want outer %#v", got, outerDecl)
	}
	if got := mustPrimitiveTypeRef(t, r, outerPrimitiveUse); got.ID != outerDecl.ID {
		t.Fatalf("outer primitive use resolved to %#v, want outer %#v", got, outerDecl)
	}
	if got := mustTypeRef(t, r, innerUse); got.ID != innerDecl.ID {
		t.Fatalf("inner use resolved to %#v, want inner %#v", got, innerDecl)
	}
	if got := mustTypeRef(t, r, afterUse); got.ID != outerDecl.ID {
		t.Fatalf("post-block use resolved to %#v, want outer %#v", got, outerDecl)
	}
	if _, ok := r.TypeRef(outsideBlockUse); ok {
		t.Fatalf("block-local type resolved outside its block")
	}
	if _, ok := r.TypeRef(beforeDefUse); ok {
		t.Fatalf("later type declaration resolved before its declaration")
	}
	selfDecl, ok := r.TypeDef(selfDef)
	if !ok {
		t.Fatalf("self type declaration missing")
	}
	if got := mustTypeRef(t, r, selfUse); got.ID != selfDecl.ID {
		t.Fatalf("self use resolved to %#v, want self declaration %#v", got, selfDecl)
	}
}

func TestTypeValueRefsSurviveNestedLocalFunctions(t *testing.T) {
	pointDef := &ast.TypeDefStmt{
		Name: "Point",
		Type: &ast.RecordTypeExpr{Fields: []ast.RecordFieldExpr{
			{Name: "x", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
			{Name: "y", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
		}},
	}
	dataDecl := localAssign([]string{"data"}, &ast.TableExpr{})

	firstPointCall := &ast.FuncCallExpr{
		Func: ident("Point"),
		Args: []ast.Expr{ident("data")},
	}
	firstFn := function(nil, ret(firstPointCall))
	firstDecl := localAssign([]string{"first"}, firstFn)

	nestedPointCall := &ast.FuncCallExpr{
		Func: ident("Point"),
		Args: []ast.Expr{ident("data")},
	}
	nestedFn := function(nil, ret(nestedPointCall))
	secondFn := function(nil,
		localAssign([]string{"helper"}, nestedFn),
		ret(&ast.FuncCallExpr{Func: ident("helper")}),
	)
	secondDecl := localAssign([]string{"second"}, secondFn)

	r := BindChunk([]ast.Stmt{
		pointDef,
		dataDecl,
		firstDecl,
		secondDecl,
	}, Options{})

	firstPointIdent := firstPointCall.Func.(*ast.IdentExpr)
	nestedPointIdent := nestedPointCall.Func.(*ast.IdentExpr)
	if _, ok := r.TypeValueRef(firstPointIdent); !ok {
		t.Fatalf("first Point call did not resolve as a type value")
	}
	if _, ok := r.TypeValueRef(nestedPointIdent); !ok {
		t.Fatalf("nested Point call did not resolve as a type value")
	}
	if got := mustSymbol(t, r, firstPointCall.Args[0].(*ast.IdentExpr)); got != mustLocalAt(t, r, dataDecl, 0) {
		t.Fatalf("first Point argument resolved to %d, want data local", got)
	}
	if got := mustSymbol(t, r, nestedPointCall.Args[0].(*ast.IdentExpr)); got != mustLocalAt(t, r, dataDecl, 0) {
		t.Fatalf("nested Point argument resolved to %d, want data local", got)
	}
}

func TestLocalValueShadowDoesNotBecomeTypeValue(t *testing.T) {
	pointDef := &ast.TypeDefStmt{
		Name: "Point",
		Type: &ast.RecordTypeExpr{Fields: []ast.RecordFieldExpr{
			{Name: "x", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
			{Name: "y", Type: &ast.PrimitiveTypeExpr{Name: "number"}},
		}},
	}
	dataDecl := localAssign([]string{"data"}, &ast.TableExpr{})
	localPointDecl := localAssign([]string{"Point"}, function([]string{"value"}, ret(ident("value"))))
	localPointCall := &ast.FuncCallExpr{
		Func: ident("Point"),
		Args: []ast.Expr{ident("data")},
	}
	shadowFn := function(nil,
		localPointDecl,
		ret(localPointCall),
	)

	r := BindChunk([]ast.Stmt{
		pointDef,
		dataDecl,
		localAssign([]string{"shadow"}, shadowFn),
	}, Options{})

	localPointID := mustLocalAt(t, r, localPointDecl, 0)
	if got := mustSymbol(t, r, localPointCall.Func.(*ast.IdentExpr)); got != localPointID {
		t.Fatalf("shadowed Point call resolved to %d, want local %d", got, localPointID)
	}
	if _, ok := r.TypeValueRef(localPointCall.Func.(*ast.IdentExpr)); ok {
		t.Fatalf("shadowed Point call was marked as a type value")
	}
}

func TestFunctionTypeParamsBindTypeRefs(t *testing.T) {
	paramRef := typeRef("T")
	returnRef := typeRef("T")
	fn := &ast.FunctionExpr{
		TypeParams: []ast.TypeParamExpr{{Name: "T"}},
		ParList: &ast.ParList{
			Names: []string{"value"},
			Types: []ast.TypeExpr{paramRef},
		},
		ReturnTypes: []ast.TypeExpr{returnRef},
	}

	r := BindFunction(fn, Options{})
	paramDecl := mustTypeRef(t, r, paramRef)
	returnDecl := mustTypeRef(t, r, returnRef)
	if paramDecl.Kind != TypeDeclParam {
		t.Fatalf("param ref kind = %v, want TypeDeclParam", paramDecl.Kind)
	}
	if returnDecl.ID != paramDecl.ID {
		t.Fatalf("return ref type param = %#v, want same declaration %#v", returnDecl, paramDecl)
	}
	params := r.FunctionTypeParams(fn)
	if len(params) != 1 || params[0].ID != paramDecl.ID {
		t.Fatalf("FunctionTypeParams = %#v, want %#v", params, paramDecl)
	}
}

func TestExpressionTraversal(t *testing.T) {
	obj := ident("obj")
	key := ident("key")
	callee := ident("callee")
	recv := ident("recv")
	tableKey := ident("tableKey")
	tableVal := ident("tableVal")
	arg := ident("arg")

	expr := &ast.LogicalOpExpr{
		Lhs: &ast.AttrGetExpr{
			Object:    obj,
			Key:       key,
			KeySyntax: ast.AttrKeyIndex,
		},
		Rhs: &ast.NonNilAssertExpr{Expr: &ast.CastExpr{
			Expr: &ast.FuncCallExpr{
				Func: callee,
				Args: []ast.Expr{
					&ast.FuncCallExpr{
						Receiver: recv,
						Method:   "method",
						Args: []ast.Expr{
							&ast.TableExpr{Fields: []*ast.Field{{
								Key:       tableKey,
								KeySyntax: ast.AttrKeyIndex,
								Value: &ast.ArithmeticOpExpr{
									Operator: "+",
									Lhs:      tableVal,
									Rhs:      arg,
								},
							}}},
						},
					},
				},
			},
			Type: &ast.TypeRefExpr{Path: []string{"IgnoredType"}},
		}},
	}

	r := BindChunk([]ast.Stmt{ret(expr)}, Options{})
	for _, read := range []*ast.IdentExpr{obj, key, callee, recv, tableKey, tableVal, arg} {
		id := mustSymbol(t, r, read)
		assertKind(t, r, id, symbol.Global)
		if !r.IsImplicitGlobalUse(read) {
			t.Fatalf("%q was not marked implicit global use", read.Value)
		}
	}
}
