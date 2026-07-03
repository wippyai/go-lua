package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestNormalReturnProjectLaneRegistryCoversStorageLanes(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(normalReturnProjectLanes) != len(storage) {
		t.Fatalf("normal-return project lane count = %d, want storage lane count %d", len(normalReturnProjectLanes), len(storage))
	}
	for _, lane := range normalReturnProjectLanes {
		if lane.project == nil {
			t.Fatal("normal-return project lane has no project function")
		}
	}
}
