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
	canonicalAxes []axisRuntimeAxis
	byID          map[string]axisRuntimeAxis
	reducers      []reducerEntry

	bottomSlots []slot

	bottomOnce sync.Once
	bottom     Value
}

type axisRuntimeAxis struct {
	spec      axis.ErasedSpec
	id        string
	keyHash   uint64
	topAny    any
	topType   reflect.Type
	bottomAny any
}

var registryRuntimeCache = struct {
	mu    sync.Mutex
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

	registryRuntimeCache.mu.Lock()
	if rt, ok := registryRuntimeCache.byReg[reg]; ok {
		registryRuntimeCache.mu.Unlock()
		return rt
	}
	registryRuntimeCache.mu.Unlock()

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
	rt.byID = make(map[string]axisRuntimeAxis, specs.Len())
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
		rt.byID[id] = meta
	}
	rt.canonicalAxes = append(rt.canonicalAxes, rt.axes...)
	sort.Slice(rt.canonicalAxes, func(i, j int) bool {
		return rt.canonicalAxes[i].id < rt.canonicalAxes[j].id
	})
	for i := range rt.canonicalAxes {
		meta := rt.canonicalAxes[i]
		rt.bottomSlots = append(rt.bottomSlots, slot{key: meta.id, value: meta.bottomAny})
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
	info, ok := rt.byID[id]
	return info, ok
}
