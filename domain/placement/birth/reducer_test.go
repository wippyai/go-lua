package birth

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/structure"
	"github.com/wippyai/go-lua/domain/value"
)

func TestReducersRefuseUnauthenticatedCandidates(t *testing.T) {
	if _, outcome := Fresh(value.FreshResultCall{}, value.Value{}); outcome != structure.Refuse {
		t.Fatalf("zero fresh candidate outcome = %v, want refuse", outcome)
	}
	if _, outcome := Allocation(nil, value.Value{}); outcome != structure.Refuse {
		t.Fatalf("nil allocation candidate outcome = %v, want refuse", outcome)
	}
}
