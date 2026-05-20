package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
)

func TestFlowEvidenceIsZero(t *testing.T) {
	if !(FlowEvidence{}).IsZero() {
		t.Fatal("empty evidence should be zero")
	}
	if (FlowEvidence{Calls: []CallEvidence{{Point: cfg.Point(1)}}}).IsZero() {
		t.Fatal("call evidence should make product non-zero")
	}
	if (FlowEvidence{ParameterUses: []ParameterUseEvidence{{Symbol: cfg.SymbolID(1), Whole: true}}}).IsZero() {
		t.Fatal("parameter-use evidence should make product non-zero")
	}
}
