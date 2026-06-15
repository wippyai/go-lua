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
	reducers      []axis.Reducer

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
		rt.reducers = make([]axis.Reducer, 0, reducers.Len())
		for i := 0; i < reducers.Len(); i++ {
			rt.reducers = append(rt.reducers, reducers.At(i))
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

func (rt *registryRuntime) canonicalSlots(slots []slot) []slot {
	if len(slots) == 0 {
		return nil
	}
	if rt.slotsAlreadyCanonical(slots) {
		return slots
	}
	byKey := make(map[string]any, len(slots))
	for _, slot := range slots {
		if slot.key == presence.Key.ID() {
			panic("product: presence is a core lane, not a sparse axis")
		}
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.IsTopAny(slot.value) {
			delete(byKey, slot.key)
			continue
		}
		byKey[slot.key] = slot.value
	}
	if len(byKey) == 0 {
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]slot, 0, len(keys))
	for _, key := range keys {
		out = append(out, slot{key: key, value: byKey[key]})
	}
	return out
}

func (rt *registryRuntime) slotsAlreadyCanonical(slots []slot) bool {
	var prev string
	for i, slot := range slots {
		if slot.key == presence.Key.ID() {
			panic("product: presence is a core lane, not a sparse axis")
		}
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.IsTopAny(slot.value) {
			return false
		}
		if i > 0 && prev >= slot.key {
			return false
		}
		prev = slot.key
	}
	return true
}

func (rt *registryRuntime) validateValue(v Value) {
	if v.n == nil {
		return
	}
	if v.n.reg != rt.reg {
		panic("product: value belongs to a different registry")
	}
	for _, slot := range v.n.slots {
		if _, ok := rt.axis(slot.key); !ok {
			panic("product: value contains slot outside registry: " + slot.key)
		}
	}
}

func (rt *registryRuntime) axisValue(info axisRuntimeAxis, v Value) any {
	if v.n != nil {
		for _, slot := range v.n.slots {
			if slot.key == info.id {
				return slot.value
			}
		}
	}
	return info.topAny
}

func (rt *registryRuntime) stableHash(shape Shape, p presence.Value, slots []slot) uint64 {
	h := internal.FnvString("value.product")
	h = internal.MixHash(h, uint64(shape)+1)
	h = internal.MixHash(h, presence.Value.Hash(p))
	for _, slot := range slots {
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		h = internal.MixHash(h, info.keyHash)
		h = internal.MixHash(h, info.spec.HashAny(slot.value))
	}
	return h
}

func (rt *registryRuntime) sameNode(n *node, shape Shape, p presence.Value, slots []slot) bool {
	if n.reg != rt.reg || n.shape != shape || !presence.Equal(n.presence, p) || len(n.slots) != len(slots) {
		return false
	}
	for i, left := range n.slots {
		right := slots[i]
		if left.key != right.key {
			return false
		}
		info, ok := rt.axis(left.key)
		if !ok || !info.spec.EqualAny(left.value, right.value) {
			return false
		}
	}
	return true
}

func (rt *registryRuntime) isProductBottom(p presence.Value, slots []slot) bool {
	if presence.Equal(p, presence.Bottom()) {
		return true
	}
	for _, slot := range slots {
		info, ok := rt.axis(slot.key)
		if !ok {
			panic("product: unregistered axis slot " + slot.key)
		}
		if info.spec.EqualAny(slot.value, info.bottomAny) {
			return true
		}
	}
	return false
}
