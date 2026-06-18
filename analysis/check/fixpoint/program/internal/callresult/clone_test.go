package callresult

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/symbol"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

// The key-map clone helpers delegate to mapedit.Clone, whose nil-for-empty
// result is load-bearing: the provider threads these maps where a nil and an
// empty map are treated as the same absence. These regressions pin both the
// nil/empty -> nil contract and the independent-copy contract.
func TestCloneKeyMapWrappersReturnNilForNilAndEmpty(t *testing.T) {
	if got := cloneFunctionKeys(nil); got != nil {
		t.Errorf("cloneFunctionKeys(nil) = %#v, want nil", got)
	}
	if got := cloneFunctionKeys(map[symbol.ID]summary.SummaryKey{}); got != nil {
		t.Errorf("cloneFunctionKeys(empty) = %#v, want nil", got)
	}
	if got := cloneFunctionExpressionKeys(map[factflow.ExprRef]summary.SummaryKey{}); got != nil {
		t.Errorf("cloneFunctionExpressionKeys(empty) = %#v, want nil", got)
	}
	if got := cloneFunctionIdentityKeys(map[identity.ID]summary.SummaryKey{}); got != nil {
		t.Errorf("cloneFunctionIdentityKeys(empty) = %#v, want nil", got)
	}
	if got := clonePathKeys(map[pathdom.PathKey]summary.SummaryKey{}); got != nil {
		t.Errorf("clonePathKeys(empty) = %#v, want nil", got)
	}
	if got := cloneFunctionTypes(map[summary.SummaryKey]*typ.Function{}); got != nil {
		t.Errorf("cloneFunctionTypes(empty) = %#v, want nil", got)
	}
}

func TestCloneKeyMapWrappersCopyEntriesIndependently(t *testing.T) {
	var key summary.SummaryKey

	functionKeys := map[symbol.ID]summary.SummaryKey{symbol.ID(7): key}
	clonedFunctionKeys := cloneFunctionKeys(functionKeys)
	if len(clonedFunctionKeys) != 1 || clonedFunctionKeys[symbol.ID(7)] != key {
		t.Fatalf("cloneFunctionKeys lost entry: %#v", clonedFunctionKeys)
	}
	delete(clonedFunctionKeys, symbol.ID(7))
	if len(functionKeys) != 1 {
		t.Fatal("cloneFunctionKeys is not independent: mutating the clone changed the source")
	}

	exprKeys := map[factflow.ExprRef]summary.SummaryKey{factflow.ExprRef(3): key}
	if got := cloneFunctionExpressionKeys(exprKeys); len(got) != 1 || got[factflow.ExprRef(3)] != key {
		t.Fatalf("cloneFunctionExpressionKeys lost entry: %#v", got)
	}

	pathKeys := map[pathdom.PathKey]summary.SummaryKey{pathdom.PathKey("root"): key}
	if got := clonePathKeys(pathKeys); len(got) != 1 || got[pathdom.PathKey("root")] != key {
		t.Fatalf("clonePathKeys lost entry: %#v", got)
	}
}
