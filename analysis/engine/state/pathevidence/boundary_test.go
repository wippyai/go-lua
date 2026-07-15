package pathevidence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
)

func TestApplyBoundaryBottomAbsorbsIndependentlyForAllFourSublanes(t *testing.T) {
	destination := Lane{
		refinementsBottom:              true,
		staticMembersBottom:            true,
		proofsBottom:                   true,
		pathPresenceImplicationsBottom: true,
	}
	got := destination.ApplyBoundary(Lane{}.Reachable(), func(keyspace.Key) bool { return true })
	if !got.RefinementsBottom() || !got.StaticMembersBottom() || !got.ProofsBottom() || !got.PathPresenceImplicationsBottom() {
		t.Fatalf("Bottom was revived during boundary apply: %#v", got)
	}
}
