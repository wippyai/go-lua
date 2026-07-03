package factapply

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestNormalReturnApplyLaneRegistryCoversStorageLanes(t *testing.T) {
	storage := callboundary.NormalReturnFactLanes()
	if len(normalReturnApplyLanes) != len(storage) {
		t.Fatalf("normal-return apply lane count = %d, want storage lane count %d", len(normalReturnApplyLanes), len(storage))
	}
	for _, lane := range normalReturnApplyLanes {
		if lane.apply == nil {
			t.Fatal("normal-return apply lane has no apply function")
		}
	}
}

func TestNormalReturnApplyLanesUseCallBoundaryPathBindings(t *testing.T) {
	for _, file := range []string{"normal_return_apply_lanes.go", "call_outcome_apply.go", "call_return_slot_facts.go"} {
		srcBytes, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		src := string(srcBytes)
		for _, forbidden := range []string{
			"substituteCallBoundaryPath",
			"callBoundaryReturnSlotIndex",
			"callBoundaryConcreteSymbolPath",
			"ctx.bindings",
			"ctx.returnBindings",
		} {
			if strings.Contains(src, forbidden) {
				t.Fatalf("%s contains %q; normal-return apply lanes must use callboundary.PathBindings for boundary paths", file, forbidden)
			}
		}
	}
}
