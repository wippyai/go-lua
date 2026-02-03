package returns

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
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
		result := CollectCalledNestedFieldAssignments(nil, nil, nil)
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
