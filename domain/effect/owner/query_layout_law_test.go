package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/schema/query"
	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/effect/owner"
)

// TestExactShapeFollowsTheRegistration states that the wire shape this family
// publishes is its registration's and nothing else's. The family a layout
// carries is the family the registration is declared under - one authored key
// today, and this is what keeps it from drifting into two - and the keying is
// the fold, not a second declaration beside the codec.
func TestExactShapeFollowsTheRegistration(t *testing.T) {
	spec := owner.QuerySpec()
	if spec.Family != factor.ExactResultFamily {
		t.Fatalf("registered family = %q, want the domain's one family key", spec.Family)
	}
	shape, shapeOK := query.NewShape(spec.Family, spec.Fold)
	if !shapeOK || shape.Family() != factor.ExactResultFamily {
		t.Fatalf("shape family = %q/%v", shape.Family(), shapeOK)
	}
	// An exact family answers its subject whole at one point, so its row carries
	// no coordinate: restating the query site's identity on the wire would
	// publish a second authority for it.
	if shape.Keyed() {
		t.Fatalf("a %v family derived a keyed answer", spec.Fold)
	}
	columns := factor.ExactResultColumns()
	if len(columns) != 2 || columns[factor.ExactColumnAtoms].Carrier.Width() != 0 {
		t.Fatal("the effect-exact family declares no atom vector column")
	}
	if !factor.ExactResultStates.Available() {
		t.Fatal("the effect-exact family names no declared row state vocabulary")
	}
}
