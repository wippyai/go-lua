package factapply

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// CovariantExposureStep is one immutable N6 exposure. The factflow payload
// owns a detached source path, and Exposure returns another detached value so
// callers cannot mutate transaction storage.
type CovariantExposureStep struct {
	exposure factflow.CovariantExposure
}

func (s CovariantExposureStep) Exposure() factflow.CovariantExposure {
	return factflow.NewCovariantExposure(s.exposure.SourcePath(), s.exposure.WideValue(), s.exposure.Kind())
}

// CovariantExposureTransaction is the complete immutable N6 finalizer program
// for one CFG point. It owns only ordered semantic syntax; State, stable path
// visibility, type-widening authority, cancellation, and solve-local context
// belong to its executor.
type CovariantExposureTransaction struct {
	point cfg.Point
	steps []CovariantExposureStep
}

// PlanCovariantExposureTransaction freezes the exact factflow exposure order
// consumed by the canonical N6 finalizer.
func PlanCovariantExposureTransaction(facts factflow.Facts, point cfg.Point) CovariantExposureTransaction {
	exposures := facts.CovariantExposures(point)
	steps := make([]CovariantExposureStep, len(exposures))
	for index, exposure := range exposures {
		steps[index] = CovariantExposureStep{exposure: exposure}
	}
	return CovariantExposureTransaction{point: point, steps: steps}
}

func (t CovariantExposureTransaction) Point() cfg.Point { return t.point }
func (t CovariantExposureTransaction) Len() int         { return len(t.steps) }
func (t CovariantExposureTransaction) Clone() CovariantExposureTransaction {
	out := CovariantExposureTransaction{point: t.point, steps: make([]CovariantExposureStep, len(t.steps))}
	for index, step := range t.steps {
		out.steps[index] = CovariantExposureStep{exposure: step.Exposure()}
	}
	return out
}

func (t CovariantExposureTransaction) HasStateSteps() bool { return len(t.steps) != 0 }

// Step returns one immutable transaction member without exposing the backing
// slice or the payload's source-path storage.
func (t CovariantExposureTransaction) Step(index int) (CovariantExposureStep, bool) {
	if index < 0 || index >= len(t.steps) {
		return CovariantExposureStep{}, false
	}
	step := t.steps[index]
	return CovariantExposureStep{exposure: step.Exposure()}, true
}

// Valid reports whether every embedded product belongs to reg. Source-path and
// kind edge cases deliberately remain executable: the concrete N6 primitive
// historically treats a zero source as a no-op and every non-array kind as the
// record lane, so rejecting either here would change established behavior.
func (t CovariantExposureTransaction) Valid(reg *axis.Registry) bool {
	if reg == nil {
		return false
	}
	for _, step := range t.steps {
		if !product.BelongsToRegistry(reg, step.exposure.WideValue()) {
			return false
		}
	}
	return true
}
