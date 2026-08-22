package owner_test

import (
	"testing"

	"github.com/wippyai/go-lua/domain/effect/factor"
	"github.com/wippyai/go-lua/domain/effect/owner"
)

// TestExactLayoutNamesTheFamilyItPublishes states that the family a result
// layout carries is the family that layout is registered under. The two are one
// authored key today; this law is what keeps them from drifting into two.
func TestExactLayoutNamesTheFamilyItPublishes(t *testing.T) {
	layout := owner.ExactResultLayout()
	if !layout.Available() {
		t.Fatal("the effect-exact layout did not seal")
	}
	if layout.Family() != owner.QuerySpec().Family {
		t.Fatalf("layout family = %q, registered family = %q", layout.Family(), owner.QuerySpec().Family)
	}
	if layout.Family() != factor.ExactResultFamily {
		t.Fatalf("layout family = %q, want the domain's one family key", layout.Family())
	}
}
