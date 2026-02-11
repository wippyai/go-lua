package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/cfg"
	checkcallsite "github.com/wippyai/go-lua/compiler/check/callsite"
)

func TestHasLocalCallSites(t *testing.T) {
	t.Run("nil graph returns false", func(t *testing.T) {
		if HasLocalCallSites(nil, nil) {
			t.Error("expected false")
		}
	})

	t.Run("empty local funcs returns false", func(t *testing.T) {
		if HasLocalCallSites(nil, map[cfg.SymbolID]*LocalFuncInfo{}) {
			t.Error("expected false")
		}
	})
}

func TestCollectCalledNestedFieldAssignments(t *testing.T) {
	t.Run("nil graph returns empty map", func(t *testing.T) {
		result := CollectCalledNestedFieldAssignments(nil, nil, nil, nil)
		if len(result) != 0 {
			t.Error("expected empty result")
		}
	})
}

func TestCollectCalledNestedContainerMutatorAssignments(t *testing.T) {
	t.Run("nil graph returns empty slice", func(t *testing.T) {
		result := CollectCalledNestedContainerMutatorAssignments(nil, nil, nil, nil)
		if len(result) != 0 {
			t.Error("expected empty result")
		}
	})
}

func TestRuntimeArgAt(t *testing.T) {
	t.Run("direct call positional mapping", func(t *testing.T) {
		a := &ast.NumberExpr{Value: "1"}
		b := &ast.NumberExpr{Value: "2"}
		info := &cfg.CallInfo{Args: []ast.Expr{a, b}}
		if got := checkcallsite.RuntimeArgAt(info, 0); got != a {
			t.Fatal("expected first arg at index 0")
		}
		if got := checkcallsite.RuntimeArgAt(info, -1); got != b {
			t.Fatal("expected last arg at index -1")
		}
	})

	t.Run("method call runtime mapping", func(t *testing.T) {
		recv := &ast.IdentExpr{Value: "self"}
		a := &ast.NumberExpr{Value: "1"}
		b := &ast.NumberExpr{Value: "2"}
		info := &cfg.CallInfo{
			Method:   "m",
			Receiver: recv,
			Args:     []ast.Expr{a, b},
		}
		if got := checkcallsite.RuntimeArgAt(info, 0); got != recv {
			t.Fatal("expected receiver at index 0 for method call")
		}
		if got := checkcallsite.RuntimeArgAt(info, 1); got != a {
			t.Fatal("expected first positional arg at runtime index 1")
		}
		if got := checkcallsite.RuntimeArgAt(info, -3); got != recv {
			t.Fatal("expected receiver from negative runtime index")
		}
	})
}
