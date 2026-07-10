package program

import (
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func seedEntryHeapObjectsForValue(reg *axis.Registry, caller state.State, entry state.State, value product.Value) (state.State, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return entry, false
	}
	return seedEntryHeapObject(reg, caller, entry, id, make(map[identity.ID]struct{}))
}

func seedEntryHeapObject(reg *axis.Registry, caller state.State, entry state.State, id identity.ID, seen map[identity.ID]struct{}) (state.State, bool) {
	if id == (identity.ID{}) {
		return entry, false
	}
	if _, ok := seen[id]; ok {
		return entry, false
	}
	seen[id] = struct{}{}

	object := caller.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return entry, false
	}

	out := entry.WriteHeapTableObject(reg, id, object)
	changed := true
	for _, value := range object.StaticMembers() {
		var copied bool
		out, copied = seedEntryHeapObjectValue(reg, caller, out, value, seen)
		changed = changed || copied
	}
	for _, fact := range object.DynamicIndexFacts() {
		var copied bool
		out, copied = seedEntryHeapObjectValue(reg, caller, out, fact.KeyValue, seen)
		changed = changed || copied
		out, copied = seedEntryHeapObjectValue(reg, caller, out, fact.Value, seen)
		changed = changed || copied
	}
	return out, changed
}

func seedEntryHeapObjectValue(reg *axis.Registry, caller state.State, entry state.State, value product.Value, seen map[identity.ID]struct{}) (state.State, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return entry, false
	}
	return seedEntryHeapObject(reg, caller, entry, id, seen)
}

func seedEntryCallableHeapObjectsForValue(reg *axis.Registry, caller state.State, entry state.State, value product.Value) (state.State, product.Value, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return entry, product.Value{}, false
	}
	out, copied := seedEntryCallableHeapObject(reg, caller, entry, id, make(map[identity.ID]struct{}))
	if !copied {
		return entry, product.Value{}, false
	}
	return out, callableContextRootValue(reg, id), true
}

func seedEntryCallableHeapObject(reg *axis.Registry, caller state.State, entry state.State, id identity.ID, seen map[identity.ID]struct{}) (state.State, bool) {
	if id == (identity.ID{}) {
		return entry, false
	}
	if _, ok := seen[id]; ok {
		return entry, false
	}
	seen[id] = struct{}{}

	object := caller.ReadHeapTableObject(reg, id)
	if heapidentity.ObjectDomain(reg).Equal(object, heapidentity.BottomObject(reg)) {
		return entry, false
	}

	out := entry
	staticMembers := make(map[keyspace.Key]product.Value)
	for key, value := range object.StaticMembers() {
		var member product.Value
		var copied bool
		out, member, copied = callableContextMemberValue(reg, caller, out, value, seen)
		if !copied {
			continue
		}
		staticMembers[key] = member
	}
	if len(staticMembers) == 0 {
		return entry, false
	}
	root := callableContextRootValue(reg, id)
	return out.WriteHeapTableObject(reg, id, heapidentity.NewOwnedStaticTableObject(root, staticMembers)), true
}

func callableContextMemberValue(reg *axis.Registry, caller state.State, entry state.State, value product.Value, seen map[identity.ID]struct{}) (state.State, product.Value, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok {
		return entry, product.Value{}, false
	}
	if valueIsCallable(reg, value) {
		return entry, value, true
	}
	out, copied := seedEntryCallableHeapObject(reg, caller, entry, id, seen)
	if !copied {
		return entry, product.Value{}, false
	}
	return out, callableContextRootValue(reg, id), true
}

func callableContextRootValue(reg *axis.Registry, id identity.ID) product.Value {
	value := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	return product.Set(reg, value, identity.Key, identity.Singleton(id))
}

func valueIsCallable(reg *axis.Registry, value product.Value) bool {
	kind := product.Get(reg, value, runtimekind.Key)
	if !kind.IsTop() && kind.Contains(runtimekind.Function) {
		return true
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok || t == nil {
		return false
	}
	_, ok = typ.UnwrapTransparentWrappers(t).(*typ.Function)
	return ok
}
