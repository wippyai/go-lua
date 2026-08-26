package relationtarget_test

import (
	"testing"

	fixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/containment"
)

func TestPlacementContainmentTargetExecutesOwnerRouteAndRepeatedJ2Fold(t *testing.T) {
	assertTargetFact(t, "containment", fixture.New(t))
}
