package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/plane"
	"github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/owner"
)

// TestSummaryLayoutNamesTheFamilyItPublishes states that the family a result
// layout carries is the family that layout is registered under. The two are one
// authored key today; this law is what keeps them from drifting into two.
func TestSummaryLayoutNamesTheFamilyItPublishes(t *testing.T) {
	layout := owner.SummaryResultLayout()
	if !layout.Available() {
		t.Fatal("the value-summary layout did not seal")
	}
	if layout.Family() != owner.QuerySpec().Family {
		t.Fatalf("layout family = %q, registered family = %q", layout.Family(), owner.QuerySpec().Family)
	}
	if layout.Family() != value.SummaryResultFamily {
		t.Fatalf("layout family = %q, want the domain's one family key", layout.Family())
	}
	// A summary family folds over a coordinate space, so its rows are keyed and
	// fenced by the identity of the space that issued them.
	if _, declared := layout.Variable(); !declared {
		t.Fatal("the value-summary layout declares no compact image column")
	}
	if plane.Format == 0 {
		t.Fatal("the plane revision is unset")
	}
}
