package captured

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestFromParentFacts_NilParentFacts(t *testing.T) {
	result := FromParentFacts(nil, nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for nil parent facts, got %v", result)
	}
}

func TestFromParentFacts_NilChildGraph(t *testing.T) {
	result := FromParentFacts(nil, nil, 1, nil)
	if result != nil {
		t.Errorf("expected nil for nil child graph, got %v", result)
	}
}

func TestFromParentFacts_ZeroDefPoint(t *testing.T) {
	result := FromParentFacts(nil, nil, 0, nil)
	if result != nil {
		t.Errorf("expected nil for zero def point, got %v", result)
	}
}

type mockTypeFacts struct{}

func (m mockTypeFacts) EffectiveTypeAt(p cfg.Point, sym cfg.SymbolID) interface{} {
	return nil
}
