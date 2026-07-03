package projectsummary

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/callboundary"
)

func TestNormalReturnProjectLaneRegistryCoversStorageLanes(t *testing.T) {
	registered := make(map[callboundary.NormalReturnFactLaneID]struct{})
	storage := make(map[callboundary.NormalReturnFactLaneID]string)
	for _, lane := range callboundary.NormalReturnFactLanes() {
		storage[lane.ID()] = lane.FieldName()
	}
	for _, lane := range normalReturnProjectLanes {
		if lane.id == "" {
			t.Fatal("normal-return project lane with empty ID")
		}
		if lane.project == nil {
			t.Fatalf("normal-return project lane %q has no project function", lane.id)
		}
		if _, ok := storage[lane.id]; !ok {
			t.Fatalf("normal-return project lane %q has no storage lane owner", lane.id)
		}
		if _, ok := registered[lane.id]; ok {
			t.Fatalf("normal-return project lane ID %q registered more than once", lane.id)
		}
		registered[lane.id] = struct{}{}
	}
	for _, storageLane := range callboundary.NormalReturnFactLanes() {
		_, ok := registered[storageLane.ID()]
		if !ok {
			t.Fatalf("storage lane %q/%s has no project lane owner", storageLane.ID(), storageLane.FieldName())
		}
	}
}
