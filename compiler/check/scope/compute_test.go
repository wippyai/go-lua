package scope

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/typ"
)

func TestBuildPointScopesNilGraph(t *testing.T) {
	if got := BuildPointScopes(nil, nil, nil, PointScopeOptions{}); got != nil {
		t.Fatalf("BuildPointScopes(nil) = %v, want nil", got)
	}
}

func TestBuildPointScopesWithBaseScope(t *testing.T) {
	graph := cfg.Build(&ast.FunctionExpr{})
	base := New().WithType("BaseType", typ.String)

	got := BuildPointScopes(graph, base, nil, PointScopeOptions{})
	if got == nil {
		t.Fatal("BuildPointScopes returned nil")
	}
	entryScope := got[graph.Entry()]
	if entryScope == nil {
		t.Fatal("entry scope missing")
	}
	if tpe, ok := entryScope.LookupType("BaseType"); !ok || !typ.TypeEquals(tpe, typ.String) {
		t.Fatalf("entry scope BaseType = %v/%v, want string", tpe, ok)
	}
}

func TestBuildPointScopesResolvesTypeDefs(t *testing.T) {
	fn := &ast.FunctionExpr{Stmts: []ast.Stmt{
		&ast.TypeDefStmt{Name: "Name", Type: &ast.PrimitiveTypeExpr{Name: "string"}},
	}}
	graph := cfg.Build(fn)

	got := BuildPointScopes(graph, New(), func(name string, _ ast.TypeExpr, _ []ast.TypeParamExpr, _ *State) typ.Type {
		if name != "Name" {
			t.Fatalf("unexpected typedef name: %q", name)
		}
		return typ.String
	}, PointScopeOptions{})

	var found typ.Type
	for _, sc := range got {
		if sc == nil {
			continue
		}
		if tpe, ok := sc.LookupType("Name"); ok {
			found = tpe
		}
	}
	if found == nil {
		t.Fatal("type definition was not added to any scope")
	}
	alias, ok := found.(*typ.Alias)
	if !ok || !typ.TypeEquals(alias.Target, typ.String) {
		t.Fatalf("type definition = %v, want alias to string", found)
	}
}
