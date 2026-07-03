package factapply

import (
	"os"
	"strings"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestNormalReturnApplyLaneRegistryCoversStorageLanes(t *testing.T) {
	registered := make(map[callboundary.NormalReturnFactLaneID]struct{})
	storage := make(map[callboundary.NormalReturnFactLaneID]string)
	for _, lane := range callboundary.NormalReturnFactLanes() {
		storage[lane.ID()] = lane.FieldName()
	}
	for _, lane := range normalReturnApplyLanes {
		if lane.id == "" {
			t.Fatal("normal-return apply lane with empty ID")
		}
		if lane.apply == nil {
			t.Fatalf("normal-return apply lane %q has no apply function", lane.id)
		}
		if _, ok := storage[lane.id]; !ok {
			t.Fatalf("normal-return apply lane %q has no storage lane owner", lane.id)
		}
		if _, ok := registered[lane.id]; ok {
			t.Fatalf("normal-return apply lane ID %q registered more than once", lane.id)
		}
		registered[lane.id] = struct{}{}
	}
	for _, storageLane := range callboundary.NormalReturnFactLanes() {
		_, ok := registered[storageLane.ID()]
		if !ok {
			t.Fatalf("storage lane %q/%s has no apply lane owner", storageLane.ID(), storageLane.FieldName())
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
