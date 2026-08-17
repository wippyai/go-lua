package grammarproof

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/internal/grammarproof/astcodec"
)

func TestObserveSourceRequiresExactGeneratedFieldState(t *testing.T) {
	rows, err := ObserveSource("return 7", "observe.lua")
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireObservedState(rows, "NumberExpr", "Value", astcodec.FieldStateNonEmpty); err != nil {
		t.Fatal(err)
	}
	if err := RequireObservedState(rows, "NumberExpr", "Value", astcodec.FieldStateZero); err == nil {
		t.Fatal("incorrect generated field state was accepted")
	}
}
