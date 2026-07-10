package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func TestObservationCaptureValidatesEveryRecordedDependencyVersion(t *testing.T) {
	point := cfg.Point(3)
	dependency := cfg.Point(7)
	plan := ObservationPlan{
		boundaryPoints: []cfg.Point{point},
		boundarySet:    map[cfg.Point]struct{}{point: {}},
		nodePoints:     []cfg.Point{point},
		nodeSet:        map[cfg.Point]struct{}{point: {}},
	}

	valid := newObservationCapture(plan)
	valid.record(transfer.NodeObservation{
		Point:        point,
		InputVersion: 11,
		Reads:        []transfer.StateRead{{Point: dependency, Version: 19}},
	})
	valid.finalize(func(p cfg.Point) uint64 {
		switch p {
		case point:
			return 11
		case dependency:
			return 19
		default:
			return 0
		}
	})
	if valid.stats.CapturedBoundaryOutputs != 1 || valid.stats.ValidatedBoundaryOutputs != 1 {
		t.Fatalf("valid capture stats = %+v, want one captured and validated output", valid.stats)
	}
	if _, ok := valid.valid[point]; !ok {
		t.Fatal("valid capture did not retain planned output")
	}

	invalid := newObservationCapture(plan)
	invalid.record(transfer.NodeObservation{
		Point:        point,
		InputVersion: 11,
		Reads:        []transfer.StateRead{{Point: dependency, Version: 19}},
	})
	invalid.finalize(func(p cfg.Point) uint64 {
		if p == point {
			return 11
		}
		return 20 // A dynamic read changed after the captured transfer.
	})
	if invalid.stats.ValidatedBoundaryOutputs != 0 {
		t.Fatalf("invalid capture stats = %+v, want no validated output", invalid.stats)
	}
	if len(invalid.valid) != 0 {
		t.Fatal("invalid capture retained an output")
	}
}
