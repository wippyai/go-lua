package product

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
	"github.com/wippyai/go-lua/analysis/lattice"
)

// registryRuntime caches frozen-registry metadata and boxed axis values.
// Cached top/bottom values are treated as immutable once published by the
// registry, which matches the existing axis and product lattice contracts.
type registryRuntime struct {
	reg                    *axis.Registry
	err                    error
	retentionSafe          bool
	interner               *interner
	axes                   []axisRuntimeAxis
	canonicalAxes          []uint16
	byID                   map[string]uint16
	reducers               []reducerEntry
	reducerDeps            [][]int
	presenceDeps           []int
	widenMeasure           []rankComponent
	reductionMeasure       []rankComponent
	reducerWrittenOrdinals []uint16
	presenceReducerWritten bool

	bottomSlots []slot

	bottomOnce sync.Once
	bottom     Value

	canonicalCodecOnce sync.Once
	canonicalCodec     canonicalProductCodec

	domainOnce sync.Once
	domain     lattice.Lattice[Value]
}

type axisRuntimeAxis struct {
	spec           axis.ErasedSpec
	id             string
	ordinal        uint16
	keyHash        uint64
	topAny         any
	topType        reflect.Type
	bottomAny      any
	reducerWritten bool
}

func runtimeFor(reg *axis.Registry) *registryRuntime {
	if reg == nil {
		return &registryRuntime{err: fmt.Errorf("product: registry is required; construct it with RegistryWithAxes")}
	}
	if !reg.Frozen() {
		return &registryRuntime{reg: reg, err: fmt.Errorf("product: registry must be frozen before use")}
	}
	projection := reg.CompiledProduct()
	rt, ok := projection.(*registryRuntime)
	if !ok || rt == nil || rt.reg != reg {
		return &registryRuntime{
			reg: reg,
			err: fmt.Errorf("product: registry has no sealed compiled product runtime; construct it with RegistryWithAxes"),
		}
	}
	return rt
}

// Owner implements axis.CompiledProduct. The exact pointer identity is the
// construction-time ownership fence for every product operation.
func (rt *registryRuntime) Owner() *axis.Registry {
	if rt == nil {
		return nil
	}
	return rt.reg
}

func mustRuntime(reg *axis.Registry) *registryRuntime {
	rt := runtimeFor(reg)
	if rt.err != nil {
		panic(rt.err)
	}
	return rt
}

func buildRegistryRuntime(reg *axis.Registry) *registryRuntime {
	rt := &registryRuntime{reg: reg, retentionSafe: true, interner: newInterner()}
	if reg == nil {
		rt.err = fmt.Errorf("product: registry is required; construct it with RegistryWithAxes")
		return rt
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		rt.err = fmt.Errorf("product: presence is a core lane and must not be registered as a sparse axis")
		return rt
	}
	// Presence is a mandatory core lane rather than a sparse registered axis.
	// Pin it explicitly so a future mutable core representation cannot bypass
	// the sparse-axis retention inventory.
	presenceSpec := presence.Spec().Erase()
	if presenceSpec.RetentionMode() != axis.RetentionImmutable ||
		!presenceSpec.RetentionSafeAny(presence.Bottom()) ||
		!presenceSpec.RetentionSafeAny(presence.Present()) ||
		!presenceSpec.RetentionSafeAny(presence.Absent()) ||
		!presenceSpec.RetentionSafeAny(presence.Maybe()) ||
		!presenceSpec.RetentionSafeAny(presence.Top()) {
		rt.retentionSafe = false
	}

	specs := reg.SpecsView()
	rt.axes = make([]axisRuntimeAxis, 0, specs.Len())
	rt.byID = make(map[string]uint16, specs.Len())
	rt.bottomSlots = make([]slot, 0, specs.Len())
	for i := 0; i < specs.Len(); i++ {
		spec := specs.At(i)
		if err := validateProductSparseAxis(spec); err != nil {
			rt.err = err
			return rt
		}
		id := spec.ID()
		topAny := spec.TopAny()
		switch spec.RetentionMode() {
		case axis.RetentionImmutable, axis.RetentionValidated:
		default:
			rt.retentionSafe = false
		}
		if !spec.RetentionSafeAny(topAny) {
			rt.retentionSafe = false
		}
		topType := reflect.TypeOf(topAny)
		if topType == nil {
			topType = reflect.TypeOf(spec.BottomAny())
		}
		meta := axisRuntimeAxis{
			spec:      spec,
			id:        id,
			keyHash:   internal.FnvString(id),
			topAny:    topAny,
			topType:   topType,
			bottomAny: spec.BottomAny(),
		}
		rt.axes = append(rt.axes, meta)
	}
	order := make([]int, len(rt.axes))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool {
		return rt.axes[order[i]].id < rt.axes[order[j]].id
	})
	rt.canonicalAxes = make([]uint16, len(order))
	for ordinal, index := range order {
		if ordinal > int(^uint16(0)) {
			rt.err = fmt.Errorf("product: registry has too many axes")
			return rt
		}
		rt.axes[index].ordinal = uint16(ordinal)
		rt.byID[rt.axes[index].id] = uint16(index)
		rt.canonicalAxes[ordinal] = uint16(index)
		rt.bottomSlots = append(rt.bottomSlots, slot{ordinal: uint16(ordinal), value: rt.axes[index].bottomAny})
	}

	if err := rt.buildReducers(reg.ReducersView()); err != nil {
		rt.err = err
		return rt
	}
	rt.buildMeasures()
	return rt
}

func (rt *registryRuntime) buildMeasures() {
	// Shape and presence are finite core lanes. Sparse axes are ordered by their
	// canonical product ordinal, never registration order.
	widen := make([]rankComponent, 0, len(rt.axes)+2)
	widen = append(widen, rankComponent{kind: shapeRank}, rankComponent{kind: presenceRank})
	for ordinal := range rt.canonicalAxes {
		info := rt.axisOrdinal(uint16(ordinal))
		width := info.spec.WidenRankWidth()
		if width <= 0 {
			// A Product Factor may still be acyclic. Do not manufacture a rank
			// from axis names, registration order, or a precision budget.
			widen = nil
			break
		}
		for component := 0; component < width; component++ {
			widen = append(widen, rankComponent{
				kind: axisWidenRank, ordinal: uint16(ordinal), component: component,
			})
		}
	}
	rt.widenMeasure = widen

	if len(rt.reducers) == 0 {
		return
	}
	reduction := make([]rankComponent, 0, len(rt.reducerWrittenOrdinals))
	for _, ordinal := range rt.reducerWrittenOrdinals {
		info := rt.axisOrdinal(ordinal)
		for component := 0; component < info.spec.ReductionRankWidth(); component++ {
			reduction = append(reduction, rankComponent{
				kind: axisReductionRank, ordinal: ordinal, component: component,
			})
		}
	}
	if rt.presenceReducerWritten {
		reduction = append([]rankComponent{{kind: presenceReductionMeasureKind}}, reduction...)
	}
	rt.reductionMeasure = reduction
}

func (rt *registryRuntime) bottomValue() Value {
	rt.bottomOnce.Do(func() {
		rt.bottom = intern(rt.reg, ShapeBottom, presence.Bottom(), rt.bottomSlots)
	})
	return rt.bottom
}

func (rt *registryRuntime) axis(id string) (axisRuntimeAxis, bool) {
	index, ok := rt.byID[id]
	if !ok {
		return axisRuntimeAxis{}, false
	}
	return rt.axes[index], true
}

func (rt *registryRuntime) axisOrdinal(ordinal uint16) axisRuntimeAxis {
	if int(ordinal) >= len(rt.canonicalAxes) {
		panic("product: slot ordinal outside registry")
	}
	return rt.axes[rt.canonicalAxes[ordinal]]
}
