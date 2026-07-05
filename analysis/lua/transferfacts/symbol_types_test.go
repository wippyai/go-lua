package transferfacts

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/lua/typeresolve"
	"github.com/wippyai/go-lua/analysis/module/importlookup"
	"github.com/wippyai/go-lua/analysis/symbol"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
	"github.com/wippyai/go-lua/compiler/ast"
)

func TestLowerSymbolTypesKeepsParamAnnotationsWithoutSemanticResult(t *testing.T) {
	fn, bindings, built, _ := parseSemanticFunction(t, `
function handle(ch: Channel<{kind: "event", id: string}>)
	local selected = channel.select { ch:case_receive() }
end
`, "channel")
	slots := bindings.ParamSlots(fn)
	if len(slots) != 1 {
		t.Fatalf("ParamSlots = %#v, want one typed parameter", slots)
	}

	got := lowerSymbolTypes(bindings, built.Graph, nil, typeresolve.New(bindings), importlookup.Source{})
	if got == nil {
		t.Fatal("lowerSymbolTypes returned nil without semantic result")
	}
	if gotType, ok := got[slots[0].Symbol]; !ok || gotType == nil {
		t.Fatalf("symbol type for parameter %d = %v/%v, want annotation", slots[0].Symbol, gotType, ok)
	}
}

func TestAccessChainTypeProjectsStaticStringAndIntIndexes(t *testing.T) {
	element := typetable.NewRecord().
		Field("name", typ.String).
		Build()
	rootType := typetable.NewRecord().
		StaticStringIndex("payload", element).
		Field("items", typ.NewArray(element)).
		Build()

	t.Run("static string index", func(t *testing.T) {
		root := &ast.IdentExpr{Value: "root"}
		expr := attrGet(
			attrGet(root, &ast.StringExpr{Value: "payload"}, ast.AttrKeyIndex),
			&ast.StringExpr{Value: "name"},
			ast.AttrKeyDot,
		)
		assertAccessChainType(t, root, expr, rootType, typ.String)
	})

	t.Run("static integer index", func(t *testing.T) {
		root := &ast.IdentExpr{Value: "root"}
		expr := attrGet(
			attrGet(
				attrGet(root, &ast.StringExpr{Value: "items"}, ast.AttrKeyDot),
				&ast.NumberExpr{Value: "1"},
				ast.AttrKeyIndex,
			),
			&ast.StringExpr{Value: "name"},
			ast.AttrKeyDot,
		)
		assertAccessChainType(t, root, expr, rootType, typeexpr.Optional(typ.String))
	})
}

func attrGet(obj ast.Expr, key ast.Expr, syntax ast.AttrKeySyntax) *ast.AttrGetExpr {
	return &ast.AttrGetExpr{
		Object:    obj,
		Key:       key,
		KeySyntax: syntax,
	}
}

func assertAccessChainType(t *testing.T, root *ast.IdentExpr, expr ast.Expr, rootType typ.Type, want typ.Type) {
	t.Helper()
	bindings := bind.BindChunk([]ast.Stmt{&ast.ReturnStmt{Exprs: []ast.Expr{expr}}}, bind.Options{})
	rootSym, ok := bindings.SymbolOf(root)
	if !ok || rootSym == 0 {
		t.Fatalf("SymbolOf(%q) = %d/%v, want non-zero symbol", root.Value, rootSym, ok)
	}
	got, ok := accessChainType(map[symbol.ID]typ.Type{rootSym: rootType}, bindings, expr)
	if !ok {
		t.Fatal("accessChainType rejected static path")
	}
	if !typ.TypeEquals(got, want) {
		t.Fatalf("accessChainType type = %v, want %v", got, want)
	}
}
