package callorder

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/parse"
)

func ident(name string) *ast.IdentExpr {
	return &ast.IdentExpr{Value: name}
}

func call(name string, args ...ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Func: ident(name), Args: args}
}

func methodCall(receiver ast.Expr, method string, args ...ast.Expr) *ast.FuncCallExpr {
	return &ast.FuncCallExpr{Receiver: receiver, Method: method, Args: args}
}

func assertOccurrences(t *testing.T, got []Occurrence, want ...*ast.FuncCallExpr) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("occurrences = %#v, want %d calls", got, len(want))
	}
	for i, wantCall := range want {
		if got[i].ExprIndex != NoExprIndex {
			t.Fatalf("occurrence %d expr index = %d, want %d", i, got[i].ExprIndex, NoExprIndex)
		}
		if got[i].Call != wantCall {
			t.Fatalf("occurrence %d call = %s, want %s", i, callLabel(got[i].Call), callLabel(wantCall))
		}
	}
}

func assertValueOccurrences(t *testing.T, got []Occurrence, exprIndex int, want ...*ast.FuncCallExpr) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("occurrences = %#v, want %d calls", got, len(want))
	}
	for i, wantCall := range want {
		if got[i].ExprIndex != exprIndex {
			t.Fatalf("occurrence %d expr index = %d, want %d", i, got[i].ExprIndex, exprIndex)
		}
		if got[i].Call != wantCall {
			t.Fatalf("occurrence %d call = %s, want %s", i, callLabel(got[i].Call), callLabel(wantCall))
		}
	}
}

func callLabel(call *ast.FuncCallExpr) string {
	if call == nil {
		return "<nil>"
	}
	if call.Method != "" {
		return call.Method
	}
	if fn, ok := call.Func.(*ast.IdentExpr); ok {
		return fn.Value
	}
	return "<call>"
}

func TestExprOrdersNestedCallsDepthFirstLeftToRight(t *testing.T) {
	deep := call("deep")
	inner := call("inner", deep)
	sibling := call("sibling")
	outer := call("outer", inner, sibling)

	got, ok := Expr(outer, Options{})
	if !ok {
		t.Fatal("Expr rejected nested call tree")
	}
	assertOccurrences(t, got, deep, inner, sibling, outer)
}

func TestExprOrdersReceiverBeforeArguments(t *testing.T) {
	recv := call("recv")
	arg := call("arg")
	outer := methodCall(recv, "run", arg)

	got, ok := Expr(outer, Options{})
	if !ok {
		t.Fatal("Expr rejected receiver call")
	}
	assertOccurrences(t, got, recv, arg, outer)
}

func TestExprOrdersTableKeyBeforeValue(t *testing.T) {
	key := call("key")
	value := call("value")
	table := &ast.TableExpr{Fields: []*ast.Field{{
		Key:   key,
		Value: value,
	}}}

	got, ok := Expr(table, Options{})
	if !ok {
		t.Fatal("Expr rejected table constructor")
	}
	assertOccurrences(t, got, key, value)
}

func TestExprRejectsLogicalShortCircuitWithRuntimeCalls(t *testing.T) {
	tests := []ast.Expr{
		&ast.LogicalOpExpr{Operator: "and", Lhs: call("lhs"), Rhs: ident("rhs")},
		&ast.LogicalOpExpr{Operator: "or", Lhs: ident("lhs"), Rhs: call("rhs")},
	}
	for i, expr := range tests {
		if got, ok := Expr(expr, Options{}); ok || got != nil {
			t.Fatalf("case %d Expr = %#v/%v, want nil/false", i, got, ok)
		}
	}
}

func TestValueListAllowsLogicalShortCircuitCallsWhenOptedIn(t *testing.T) {
	makeCall := call("make")
	cachedOrMake := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      ident("cached"),
		Rhs:      makeCall,
	}
	guardedCall := call("make")
	guardAndMake := &ast.LogicalOpExpr{
		Operator: "and",
		Lhs:      ident("guard"),
		Rhs:      guardedCall,
	}
	lhsCall := call("cached")
	callOrFallback := &ast.LogicalOpExpr{
		Operator: "or",
		Lhs:      lhsCall,
		Rhs:      ident("fallback"),
	}
	options := Options{AllowShortCircuitCalls: true}

	got, ok := ValueList([]ast.Expr{cachedOrMake}, options)
	if !ok {
		t.Fatal("ValueList rejected cached or make() with short-circuit calls enabled")
	}
	assertValueOccurrences(t, got, 0, makeCall)

	got, ok = ValueList([]ast.Expr{guardAndMake}, options)
	if !ok {
		t.Fatal("ValueList rejected guard and make() with short-circuit calls enabled")
	}
	assertValueOccurrences(t, got, 0, guardedCall)

	got, ok = ValueList([]ast.Expr{callOrFallback}, options)
	if !ok {
		t.Fatal("ValueList rejected call() or fallback with short-circuit calls enabled")
	}
	assertValueOccurrences(t, got, 0, lhsCall)
}

func TestExprUnwrapsAssertionWrappers(t *testing.T) {
	wrapped := []ast.Expr{
		&ast.CastExpr{Expr: call("casted"), Type: &ast.PrimitiveTypeExpr{Name: "number"}},
		&ast.NonNilAssertExpr{Expr: call("asserted")},
		&ast.NonNilAssertExpr{Expr: &ast.CastExpr{Expr: call("nested"), Type: &ast.PrimitiveTypeExpr{Name: "any"}}},
	}
	for _, expr := range wrapped {
		got, ok := Expr(expr, Options{})
		if !ok {
			t.Fatalf("Expr(%T) rejected wrapped call", expr)
		}
		if len(got) != 1 || got[0].Call == nil {
			t.Fatalf("Expr(%T) = %#v, want one wrapped call", expr, got)
		}
	}
}

func TestExprSuppressesExpressionCoveredPredicates(t *testing.T) {
	callExpr := call("type", ident("value"))
	cond := &ast.RelationalOpExpr{
		Operator: "==",
		Lhs:      callExpr,
		Rhs:      &ast.StringExpr{Value: "string"},
	}
	stmts := []ast.Stmt{
		&ast.LocalAssignStmt{Names: []string{"value"}, Exprs: []ast.Expr{&ast.NumberExpr{Value: "1"}}},
		&ast.ReturnStmt{Exprs: []ast.Expr{cond}},
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"type"}})

	got, ok := Expr(cond, SealedLuaTypeOptions(bindings))
	if !ok {
		t.Fatal("Expr rejected expression-covered predicate")
	}
	if len(got) != 0 {
		t.Fatalf("Expr = %#v, want no runtime calls for covered predicate", got)
	}
}

func TestExprKeepsChannelSelectOpaque(t *testing.T) {
	src := `
local events_ch: Channel<string>
local stop_ch: Channel<string>
local result = channel.select { events_ch:case_receive(), stop_ch:case_receive() }
`
	stmts, err := parse.ParseString(src, "test")
	if err != nil {
		t.Fatalf("ParseString: %v", err)
	}
	bindings := bind.BindChunk(stmts, bind.Options{Globals: []string{"channel"}})
	local, ok := stmts[len(stmts)-1].(*ast.LocalAssignStmt)
	if !ok {
		t.Fatalf("last stmt = %T, want *ast.LocalAssignStmt", stmts[len(stmts)-1])
	}
	selectCall, ok := local.Exprs[0].(*ast.FuncCallExpr)
	if !ok {
		t.Fatalf("local expr = %T, want *ast.FuncCallExpr", local.Exprs[0])
	}

	got, ok := Expr(selectCall, LuaOptions(bindings))
	if !ok {
		t.Fatal("Expr rejected channel.select call")
	}
	assertOccurrences(t, got, selectCall)
}
