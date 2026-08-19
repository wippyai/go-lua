package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema/cold"
)

func TestCallTargetRowsRequireAllBoundaryIdentities(t *testing.T) {
	complete := cold.CallTarget{
		Allocation: identity.ContentID{1}, Body: identity.ContentID{2}, Context: identity.ContentID{3},
		Function: identity.ContentID{4}, Formal: identity.ContentID{5},
	}
	row := CallTargetRow{row: complete}
	if !row.Available() || row.AllocationID() != (identity.ContentID{1}) {
		t.Fatal("complete call target row unavailable")
	}

	missingFormal := complete
	missingFormal.Formal = identity.ContentID{}
	if (CallTargetRow{row: missingFormal}).Available() {
		t.Fatal("call target admitted missing formal boundary")
	}
	for name, mutate := range map[string]func(*cold.CallTarget){
		"allocation": func(row *cold.CallTarget) { row.Allocation = identity.ContentID{} },
		"body":       func(row *cold.CallTarget) { row.Body = identity.ContentID{} },
		"context":    func(row *cold.CallTarget) { row.Context = identity.ContentID{} },
		"function":   func(row *cold.CallTarget) { row.Function = identity.ContentID{} },
	} {
		incomplete := complete
		mutate(&incomplete)
		if (CallTargetRow{row: incomplete}).Available() {
			t.Fatalf("call target admitted missing %s", name)
		}
	}
}
