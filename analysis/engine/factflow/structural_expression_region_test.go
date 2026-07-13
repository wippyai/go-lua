package factflow

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestStructuralExpressionRegionIsCanonicalAndImmutable(t *testing.T) {
	sourcePoints := []cfg.Point{11, 7, 8, 7}
	region, ok := NewStructuralExpressionRegion(2, 7, 12, 12, true, sourcePoints)
	if !ok {
		t.Fatal("valid region rejected")
	}
	sourcePoints[0] = 99
	points := region.OwnedRHSPoints()
	points[0] = 88
	if !reflect.DeepEqual(region.OwnedRHSPoints(), []cfg.Point{7, 8, 11}) {
		t.Fatalf("region points = %v", region.OwnedRHSPoints())
	}
}

func TestStructuralExpressionRegionRejectsIncompleteMetadata(t *testing.T) {
	tests := []struct {
		name                  string
		branch, yes, no, join cfg.Point
		rhsOnTrue             bool
		points                []cfg.Point
	}{
		{name: "same targets", branch: 2, yes: 3, no: 3, join: 4, rhsOnTrue: true, points: []cfg.Point{3}},
		{name: "missing rhs target", branch: 2, yes: 3, no: 4, join: 4, rhsOnTrue: true, points: []cfg.Point{5}},
		{name: "branch owned", branch: 2, yes: 3, no: 4, join: 4, rhsOnTrue: true, points: []cfg.Point{2, 3}},
		{name: "join owned", branch: 2, yes: 3, no: 4, join: 4, rhsOnTrue: true, points: []cfg.Point{3, 4}},
		{name: "bypass misses join", branch: 2, yes: 3, no: 5, join: 4, rhsOnTrue: true, points: []cfg.Point{3}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, ok := NewStructuralExpressionRegion(test.branch, test.yes, test.no, test.join, test.rhsOnTrue, test.points); ok {
				t.Fatal("malformed region accepted")
			}
		})
	}
}
