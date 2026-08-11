package product

import (
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

// Measure is a frozen product-owned lexicographic descent witness.  It keeps
// axis values opaque: the owning axis supplies every component, while Product
// supplies only the canonical axis order and sparse-Top interpretation.
//
// It is intentionally compatible with engine.Measure.At without importing the
// engine package. A domain adapts it at Factor declaration; the engine never
// learns which axes, ranks, or reductions it contains.
type Measure struct {
	runtime    *registryRuntime
	components []rankComponent
}

type rankKind uint8

const (
	shapeRank rankKind = iota
	presenceRank
	presenceReductionMeasureKind
	axisWidenRank
	axisReductionRank
)

type rankComponent struct {
	kind      rankKind
	ordinal   uint16
	component int
}

// WidenMeasure returns the complete product widening witness exactly when
// every sparse axis has declared one. An incomplete registry remains usable
// for acyclic Factors, but is ineligible for a cyclic Product Factor.
func WidenMeasure(reg *axis.Registry) (Measure, bool) {
	runtime := runtimeFor(reg)
	if runtime.err != nil || len(runtime.widenMeasure) == 0 {
		return Measure{}, false
	}
	return Measure{runtime: runtime, components: runtime.widenMeasure}, true
}

// ReductionMeasure returns the closure-descent witness over exactly the
// Product lanes reducers can write. It exists only after reducer registration
// has proved a ReductionRank for each such lane.
func ReductionMeasure(reg *axis.Registry) (Measure, bool) {
	runtime := runtimeFor(reg)
	if runtime.err != nil || len(runtime.reductionMeasure) == 0 {
		return Measure{}, false
	}
	return Measure{runtime: runtime, components: runtime.reductionMeasure}, true
}

// Width is the fixed arity of the frozen lexicographic witness.
func (measure Measure) Width() int { return len(measure.components) }

// At returns one component of v's frozen witness. It allocates nothing and
// treats omitted sparse slots as the owning axis's Top value.
func (measure Measure) At(v Value, component int) uint64 {
	if measure.runtime == nil || component < 0 || component >= len(measure.components) {
		panic("product: unavailable measure component")
	}
	if v.n != nil && v.n.reg != measure.runtime.reg {
		panic("product: measure value belongs to a different registry")
	}
	entry := measure.components[component]
	switch entry.kind {
	case shapeRank:
		return shapeWidenRank(ShapeOf(v))
	case presenceRank:
		return presence.Spec().WidenRank.At(PresenceOf(v), entry.component)
	case presenceReductionMeasureKind:
		return presence.Spec().ReductionRank.At(PresenceOf(v), entry.component)
	case axisWidenRank, axisReductionRank:
		axisInfo := measure.runtime.axisOrdinal(entry.ordinal)
		value, ok := lookupSlot(v, entry.ordinal)
		if !ok {
			value = axisInfo.topAny
		}
		if entry.kind == axisWidenRank {
			return axisInfo.spec.WidenRankAtAny(value, entry.component)
		}
		return axisInfo.spec.ReductionRankAtAny(value, entry.component)
	default:
		panic(fmt.Sprintf("product: invalid measure component kind %d", entry.kind))
	}
}

func shapeWidenRank(shape Shape) uint64 {
	switch shape {
	case ShapeBottom:
		return 1
	case ShapeTop:
		return 0
	default:
		panic("product: invalid shape rank value")
	}
}
