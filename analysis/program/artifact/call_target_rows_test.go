package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestCallTargetRowsRequireAllBoundaryIdentities(t *testing.T) {
	row := CallTargetRow{allocation: identity.ContentID{1}, body: identity.ContentID{2}, context: identity.ContentID{3}, function: identity.ContentID{4}, formal: identity.ContentID{5}, sealed: true}
	if !row.Available() || row.AllocationID() != (identity.ContentID{1}) {
		t.Fatal("complete call target row unavailable")
	}
	row.formal = identity.ContentID{}
	if row.Available() {
		t.Fatal("call target admitted missing formal boundary")
	}
}
