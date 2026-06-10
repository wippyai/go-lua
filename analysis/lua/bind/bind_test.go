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

	gotNames := PredeclaredGlobalNames(map[string]int{"": 1, "b": 2, "a": 3})
	if want := []string{"a", "b"}; !reflect.DeepEqual(gotNames, want) {
		t.Fatalf("PredeclaredGlobalNames = %v, want %v", gotNames, want)
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
