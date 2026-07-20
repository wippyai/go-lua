package state

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/internal/mapedit"
)

func (s State) ReadDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key) dynamicindex.Fact {
	if !s.laneEnabled(laneDynamicIndexBit) {
		return dynamicindex.Domain(reg).Bottom()
	}
	return s.dynamicIndex.read(reg, key)
}

// ForEachDynamicIndexFact visits finite dynamic-index facts without cloning the
// lane. It returns true when the lane is top, in which case no finite facts are
// visited.
func (s State) ForEachDynamicIndexFact(visit func(dynamicindex.Key, dynamicindex.Fact) bool) (top bool) {
	if !s.laneEnabled(laneDynamicIndexBit) {
		return false
	}
	if s.dynamicIndex.top {
		return true
	}
	for key, fact := range s.dynamicIndex.values {
		if !visit(key, fact) {
			break
		}
	}
	return false
}

func (s State) WriteDynamicIndexFact(reg *axis.Registry, key dynamicindex.Key, fact dynamicindex.Fact) State {
	if key.Table.Kind == keyspace.KindInvalid || !s.laneEnabled(laneDynamicIndexBit) {
		return s
	}
	requireFiniteLaneForWrite(s.dynamicIndex.top, "finite-write", "dynamic index fact", "dynamic-index")
	domain := dynamicindex.Domain(reg)
	if domain.Equal(fact, domain.Bottom()) {
		facts, changed := s.dynamicIndex.without(key)
		if !changed {
			return s
		}
		out := s.reachable()
		out.dynamicIndex = facts
		return out
	}
	if domain.Equal(s.dynamicIndex.read(reg, key), fact) {
		return s
	}
	out := s.reachable()
	out.dynamicIndex = s.dynamicIndex.with(key, fact)
	return out
}

// DynamicIndexEdit batches dynamic-index fact writes against one State
// snapshot. It is equivalent to repeated WriteDynamicIndexFact calls, including
// bottom canonicalization and read-after-write behavior, but clones the
// persistent dynamic-index map at most once.
type DynamicIndexEdit struct {
	state   State
	reg     *axis.Registry
	enabled bool
	changed bool
	cloned  bool
	values  map[dynamicindex.Key]dynamicindex.Fact
}

// EditDynamicIndex opens a dynamic-index edit transaction. Call Done or DoneOn
// to publish staged facts.
func (s State) EditDynamicIndex(reg *axis.Registry) DynamicIndexEdit {
	return DynamicIndexEdit{
		state:   s,
		reg:     reg,
		enabled: s.laneEnabled(laneDynamicIndexBit),
	}
}

// Read returns the current fact for key, including writes already staged in
// this edit transaction.
func (e *DynamicIndexEdit) Read(key dynamicindex.Key) dynamicindex.Fact {
	if e == nil {
		return dynamicindex.Fact{}
	}
	if key.Table.Kind == keyspace.KindInvalid || !e.enabled {
		return dynamicindex.Bottom(e.reg)
	}
	if e.state.dynamicIndex.top {
		return dynamicindex.Top()
	}
	if e.cloned {
		if fact, ok := e.values[key]; ok {
			return fact
		}
		return dynamicindex.Bottom(e.reg)
	}
	return e.state.dynamicIndex.read(e.reg, key)
}

// Write stages a dynamic-index fact write. Writing bottom removes the finite
// entry, preserving the map lane's canonical absence-as-bottom spelling.
func (e *DynamicIndexEdit) Write(key dynamicindex.Key, fact dynamicindex.Fact) bool {
	if e == nil || key.Table.Kind == keyspace.KindInvalid || !e.enabled {
		return false
	}
	requireFiniteLaneForWrite(e.state.dynamicIndex.top, "finite-write", "dynamic index fact", "dynamic-index")
	domain := dynamicindex.Domain(e.reg)
	current := e.Read(key)
	if domain.Equal(current, fact) {
		return false
	}
	e.ensureValues()
	if domain.Equal(fact, domain.Bottom()) {
		delete(e.values, key)
	} else {
		e.values[key] = fact
	}
	e.changed = true
	return true
}

// Done publishes staged dynamic-index facts onto the original edit state.
func (e *DynamicIndexEdit) Done() State {
	if e == nil {
		return State{}
	}
	return e.DoneOn(e.state)
}

// DoneOn publishes staged dynamic-index facts onto base. Callers must ensure no
// independent dynamic-index writes were made to base while the edit was open.
func (e *DynamicIndexEdit) DoneOn(base State) State {
	if e == nil || !e.enabled || !e.changed {
		return base
	}
	out := base.reachable()
	out.dynamicIndex.values = canonicalDynamicIndexFacts(e.values)
	return out
}

func (e *DynamicIndexEdit) ensureValues() {
	if e.cloned {
		return
	}
	e.values = mapedit.Clone(e.state.dynamicIndex.values)
	if e.values == nil {
		e.values = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
	}
	e.cloned = true
}

func canonicalDynamicIndexFacts(values map[dynamicindex.Key]dynamicindex.Fact) map[dynamicindex.Key]dynamicindex.Fact {
	if len(values) == 0 {
		return nil
	}
	return values
}

func (s State) ClearDynamicIndexFactsForPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) State {
	return s.clearDynamicIndexFactsForPathKey(ks, pathKey, false)
}

func (s State) ClearDynamicIndexFactsForPathKeyDescendants(ks *keyspace.KeySpace, pathKey pathdom.PathKey) State {
	return s.clearDynamicIndexFactsForPathKey(ks, pathKey, true)
}

func (s State) clearDynamicIndexFactsForPathKey(ks *keyspace.KeySpace, pathKey pathdom.PathKey, strict bool) State {
	if ks == nil || pathKey == "" || s.dynamicIndex.top || !s.laneEnabled(laneDynamicIndexBit) {
		return s
	}
	prefix, ok := ks.FromStateKey(pathKey)
	if !ok {
		return s
	}
	values, changed := mapedit.DeleteMatching(s.dynamicIndex.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool {
		matches := stateKeyHasPrefix(ks, key.Table, prefix)
		if strict {
			matches = stateKeyHasStrictPrefix(ks, key.Table, prefix)
		}
		return matches
	})
	if !changed {
		return s
	}
	out := s.reachable()
	out.dynamicIndex.values = values
	return out
}

func (l dynamicIndexLane) clearPathKeyDescendants(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (dynamicIndexLane, bool, bool) {
	if ks == nil || pathKey == "" || l.top {
		return l, false, ks != nil && pathKey != ""
	}
	prefix, ok := ks.FromStateKey(pathKey)
	if !ok {
		return l, false, false
	}
	values, changed := mapedit.DeleteMatching(l.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool {
		return stateKeyHasStrictPrefix(ks, key.Table, prefix)
	})
	if !changed {
		return l, false, true
	}
	l.values = values
	return l, true, true
}

func (l dynamicIndexLane) clearPathKeySubtree(ks *keyspace.KeySpace, pathKey pathdom.PathKey) (dynamicIndexLane, bool, bool) {
	if ks == nil || pathKey == "" || l.top {
		return l, false, ks != nil && pathKey != ""
	}
	prefix, ok := ks.FromStateKey(pathKey)
	if !ok {
		return l, false, false
	}
	values, changed := mapedit.DeleteMatching(l.values, func(key dynamicindex.Key, _ dynamicindex.Fact) bool {
		return stateKeyHasPrefix(ks, key.Table, prefix)
	})
	if !changed {
		return l, false, true
	}
	l.values = values
	return l, true, true
}
