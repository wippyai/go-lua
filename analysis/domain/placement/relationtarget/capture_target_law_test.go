package relationtarget_test

import (
	"testing"

	fixture "github.com/wippyai/go-lua/analysis/domain/placement/targetfixture/capture"
)

func TestPlacementCaptureTargetExecutesAuthenticatedRoutedInputs(t *testing.T) {
	assertTargetFact(t, "capture", fixture.New(t))
}
