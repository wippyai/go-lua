package expand_test

import (
	"testing"

	operator "github.com/wippyai/go-lua/analysis/engine/relation/operator/expand"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	arrangementexpand "github.com/wippyai/go-lua/analysis/relation/mount/arrangement/expand"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
)

// TestExecuteRefusesUnavailableEvidence proves the runtime boundary is
// fail-closed. Expand cannot turn an absent owner vector, reader, or source
// range into an empty/default result, and it has no legacy execution path to
// fall back to.
func TestExecuteRefusesUnavailableEvidence(t *testing.T) {
	if result, ok := operator.Execute(arrangementexpand.Evidence{}, witness.Mounted{}, geometry.Geometry{}, tuple.Batch{}, read.Reader{}); ok || result != nil {
		t.Fatal("unavailable Expand evidence was accepted")
	}
}
