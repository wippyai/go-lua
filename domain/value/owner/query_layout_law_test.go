package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/value"
	"github.com/wippyai/go-lua/domain/value/owner"
)

// TestSummaryShapeFollowsTheRegistration states that the wire shape this
// family publishes is its registration's and nothing else's. The family a
// layout carries is the family the registration is declared under - one
// authored key today, and this is what keeps it from drifting into two - and
// the keying is the fold, not a second declaration beside the codec.
func TestSummaryShapeFollowsTheRegistration(t *testing.T) {
	spec := owner.QuerySpec()
	if spec.Family != value.SummaryResultFamily {
		t.Fatalf("registered family = %q, want the domain's one family key", spec.Family)
	}
	shape, shapeOK := query.NewShape(spec.Family, spec.Fold)
	if !shapeOK || shape.Family() != value.SummaryResultFamily {
		t.Fatalf("shape family = %q/%v", shape.Family(), shapeOK)
	}
	// A summary family folds over a coordinate space, so its rows are keyed and
	// fenced by the identity of the space that issued them.
	if !shape.Keyed() {
		t.Fatalf("a %v family derived an unkeyed answer", spec.Fold)
	}
	// The columns the family publishes are its own statement; the row state
	// vocabulary it ranks against is the sealed surface's.
	columns := value.SummaryResultColumns()
	if len(columns) != 2 || columns[value.SummaryColumnImage].Carrier.Width() != 0 {
		t.Fatal("the value-summary family declares no compact image column")
	}
	if !value.SummaryResultStates.Available() {
		t.Fatal("the value-summary family names no declared row state vocabulary")
	}
}
