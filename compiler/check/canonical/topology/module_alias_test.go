package topology

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/io"
	"github.com/wippyai/go-lua/types/typ"
)

func TestDiscoverModuleAliasesWalksNestedGraphs(t *testing.T) {
	t.Parallel()

	nested := &ast.FunctionExpr{Stmts: []ast.Stmt{}}
	rootFn := &ast.FunctionExpr{
		Stmts: []ast.Stmt{
			&ast.LocalAssignStmt{
				Names: []string{"f"},
				Exprs: []ast.Expr{nested},
			},
		},
	}
	root := cfg.Build(rootFn)
	child := cfg.Build(nested)
	if root == nil || child == nil {
		t.Fatal("test graphs not built")
	}
	got := DiscoverModuleAliases(ModuleAliasDiscoveryInput{
		Root: root,
		GraphForFunc: func(fn *ast.FunctionExpr) *cfg.Graph {
			if fn == nested {
				return child
			}
			return nil
		},
		AliasesForGraph: func(g *cfg.Graph) map[cfg.SymbolID]string {
			switch g.ID() {
			case root.ID():
				return map[cfg.SymbolID]string{cfg.SymbolID(1): "root_mod"}
			case child.ID():
				return map[cfg.SymbolID]string{cfg.SymbolID(2): "child_mod"}
			default:
				return nil
			}
		},
	})
	if got[cfg.SymbolID(1)] != "root_mod" || got[cfg.SymbolID(2)] != "child_mod" || len(got) != 2 {
		t.Fatalf("DiscoverModuleAliases = %#v, want root and nested aliases", got)
	}
}

func TestResolveModuleAliasesUsesEnrichedExports(t *testing.T) {
	t.Parallel()

	manifest := io.NewManifest("mod")
	manifest.SetExport(typ.String)
	got := ResolveModuleAliases(
		map[cfg.SymbolID]string{
			cfg.SymbolID(3): "mod",
			cfg.SymbolID(4): "missing",
		},
		manifestMap{"mod": manifest},
	)
	if len(got) != 1 {
		t.Fatalf("ResolveModuleAliases = %#v, want one resolved alias", got)
	}
	if got[0].Symbol != cfg.SymbolID(3) || !typ.TypeEquals(got[0].Type, typ.String) {
		t.Fatalf("resolved alias = %#v, want symbol 3 string", got[0])
	}
}

type manifestMap map[string]*io.Manifest

func (m manifestMap) Manifest(path string) *io.Manifest {
	return m[path]
}

func (m manifestMap) Imports() map[string]*io.Manifest {
	out := make(map[string]*io.Manifest, len(m))
	for path, manifest := range m {
		out[path] = manifest
	}
	return out
}
