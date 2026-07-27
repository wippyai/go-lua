package state

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func TestDefaultProductAllLanesHaveTotalExactMeet(t *testing.T) {
	domain := RegisteredProductDomain(standard.Registry())
	lanes := domain.LaneInventory()
	if len(lanes) != 17 {
		t.Fatalf("default lane inventory has %d lanes, want 17", len(lanes))
	}
	for _, lane := range lanes {
		t.Run(string(lane.ID()), func(t *testing.T) {
			bottom, err := domain.LaneBottom(lane)
			if err != nil {
				t.Fatal(err)
			}
			top, err := domain.LaneTop(lane)
			if err != nil {
				t.Fatal(err)
			}
			for _, operands := range [][2]LaneFactor{{bottom, top}, {top, bottom}} {
				met, meetErr := domain.LaneMeet(operands[0], operands[1])
				if meetErr != nil {
					t.Fatalf("Meet(Bottom, Top) is not total: %v", meetErr)
				}
				equal, equalErr := domain.LaneEqual(met, bottom)
				if equalErr != nil {
					t.Fatal(equalErr)
				}
				if !equal {
					t.Fatal("Top is not the exact Meet identity")
				}
			}
		})
	}
}

func TestProductDomainRejectsLaneWithoutExactMeet(t *testing.T) {
	spec := placementLaneSpec
	spec.id = "test-missing-exact-meet"
	build := spec.build
	spec.build = func(reg *axis.Registry, options DomainOptions) laneOps {
		ops := build(reg, options)
		ops.factor.meet = nil
		return ops
	}
	catalog := newLaneCatalog([]laneSpec{spec})
	defer func() {
		if recover() == nil {
			t.Fatal("ProductDomain admitted a lane without exact Meet")
		}
	}()
	_ = catalog.ProductDomain(standard.Registry())
}

func TestProductDomainRejectsCoordinateMeetGaps(t *testing.T) {
	for _, test := range []struct {
		name   string
		remove func(*coordinateFamilyOps)
	}{
		{name: "skeleton", remove: func(ops *coordinateFamilyOps) { ops.skeletonMeet = nil }},
		{name: "scalar", remove: func(ops *coordinateFamilyOps) { ops.scalarMeet = nil }},
	} {
		t.Run(test.name, func(t *testing.T) {
			spec := placementLaneSpec
			families := append([]coordinateFamilySpec(nil), spec.coordinateFamilies...)
			build := families[0].build
			families[0].build = func(reg *axis.Registry, options DomainOptions) coordinateFamilyOps {
				ops := build(reg, options)
				test.remove(&ops)
				return ops
			}
			spec.coordinateFamilies = families
			catalog := newLaneCatalog([]laneSpec{spec})
			defer func() {
				if recover() == nil {
					t.Fatalf("ProductDomain admitted a coordinate family without exact %s Meet", test.name)
				}
			}()
			_ = catalog.ProductDomain(standard.Registry())
		})
	}
}
