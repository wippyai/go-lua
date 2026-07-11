package product

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	internal "github.com/wippyai/go-lua/analysis/internal/hash"
)

// registryRuntime caches frozen-registry metadata and boxed axis values.
// Cached top/bottom values are treated as immutable once published by the
// registry, which matches the existing axis and product lattice contracts.
type registryRuntime struct {
	reg           *axis.Registry
	err           error
	axes          []axisRuntimeAxis
	canonicalAxes []uint16
	byID          map[string]uint16
	reducers      []reducerEntry

	bottomSlots []slot

	bottomOnce sync.Once
	bottom     Value
}

type axisRuntimeAxis struct {
	spec      axis.ErasedSpec
	id        string
	ordinal   uint16
	keyHash   uint64
	topAny    any
	topType   reflect.Type
	bottomAny any
}

var registryRuntimeCache = struct {
	// Product operations consult this cache on every construction and
	// comparison. Registries are frozen before they enter it, so the common
	// lookup path is read-only and may proceed concurrently with other solver
	// workers.
	mu    sync.RWMutex
	byReg map[*axis.Registry]*registryRuntime
}{
	byReg: make(map[*axis.Registry]*registryRuntime),
}

func runtimeFor(reg *axis.Registry) *registryRuntime {
	if reg == nil {
		return &registryRuntime{err: fmt.Errorf("product: registry is required; pass a non-nil frozen registry")}
	}
	if !reg.Frozen() {
		return &registryRuntime{reg: reg, err: fmt.Errorf("product: registry must be frozen before use")}
	}

	registryRuntimeCache.mu.RLock()
	if rt, ok := registryRuntimeCache.byReg[reg]; ok {
		registryRuntimeCache.mu.RUnlock()
		return rt
	}
	registryRuntimeCache.mu.RUnlock()

	rt := buildRegistryRuntime(reg)
	if rt.err != nil {
		registryRuntimeCache.mu.Lock()
		if existing, ok := registryRuntimeCache.byReg[reg]; ok {
			registryRuntimeCache.mu.Unlock()
			return existing
		}
		registryRuntimeCache.byReg[reg] = rt
		registryRuntimeCache.mu.Unlock()
		return rt
	}

	registryRuntimeCache.mu.Lock()
	if existing, ok := registryRuntimeCache.byReg[reg]; ok {
		registryRuntimeCache.mu.Unlock()
		return existing
	}
	registryRuntimeCache.byReg[reg] = rt
	registryRuntimeCache.mu.Unlock()
	return rt
}

func mustRuntime(reg *axis.Registry) *registryRuntime {
	rt := runtimeFor(reg)
	if rt.err != nil {
		panic(rt.err)
	}
	return rt
}

func buildRegistryRuntime(reg *axis.Registry) *registryRuntime {
	rt := &registryRuntime{reg: reg}
	if reg == nil {
		rt.err = fmt.Errorf("product: registry is required; pass a non-nil frozen registry")
		return rt
	}
	if !reg.Frozen() {
		rt.err = fmt.Errorf("product: registry must be frozen before use")
		return rt
	}
	if _, ok := reg.LookupErased(presence.Key.ID()); ok {
		rt.err = fmt.Errorf("product: presence is a core lane and must not be registered as a sparse axis")
		return rt
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

	reducers := reg.ReducersView()
	if reducers.Len() > 0 {
		rt.reducers = make([]reducerEntry, 0, reducers.Len())
		for i := 0; i < reducers.Len(); i++ {
			rt.reducers = append(rt.reducers, reducerEntry{apply: reducers.At(i), reads: reducers.ReadsAt(i)})
		}
	}
	return rt
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
