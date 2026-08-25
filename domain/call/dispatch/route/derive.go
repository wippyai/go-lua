// Package route owns Call dispatch's dependent relation derivation. It is the
// only place where a dynamic Value callee is reduced through Value and Heap
// authorities into Call-owned route members. The result carries no foreign
// fact or target representation.
package route

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

const inlineWidth = 8

// Plan is one invocation's canonical dispatch relation. Ordinary call sets
// stay inline; only a proven finite set wider than the inline extent spills.
type Plan struct {
	inline [inlineWidth]calldomain.DispatchRoute
	spill  []calldomain.DispatchRoute
	count  int
}

// Derive builds the exact route relation for one mounted Call candidate and
// one Value callee fact. Missing or foreign authority refuses. Bottom and a
// finite non-callable image are valid empty relations; uncertainty is an
// explicit Call-owned opaque or top route, never a fallback.
func Derive(calls *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema, candidate calldomain.CallCoordinate, callee valuedomain.Value) (Plan, bool) {
	if calls == nil || !calls.Valid() || values == nil || !values.Valid() || !heaps.Valid() ||
		!calls.OwnsCallCoordinate(candidate) || !values.OwnsHeapSchema(heaps) ||
		!values.LinkOwner().Matches(calls.LinkOwner()) || !heaps.LinkOwner().Matches(calls.LinkOwner()) {
		return Plan{}, false
	}
	key, keyOK := candidate.Key()
	if !keyOK || !key.IsApplication() {
		return Plan{}, false
	}
	if callee.IsTop() {
		row, ok := calls.DispatchTopRoute(key)
		if !ok {
			return Plan{}, false
		}
		var plan Plan
		return plan, plan.add(row)
	}
	if callee.IsBottom() {
		return Plan{}, true
	}

	var plan Plan
	visited := values.VisitAtoms(callee, func(atom valuedomain.Atom) bool {
		row, present, ok := classify(calls, values, heaps, candidate, key, atom)
		if !ok {
			plan.count = -1
			return false
		}
		if present && !plan.add(row) {
			plan.count = -1
			return false
		}
		return true
	})
	return plan, visited && plan.count >= 0
}

func classify(calls *calldomain.Algebra, values *valuedomain.Schema, heaps heapdomain.Schema, candidate calldomain.CallCoordinate, key calldomain.Key, atom valuedomain.Atom) (calldomain.DispatchRoute, bool, bool) {
	if !values.OwnsAtom(atom) {
		return calldomain.DispatchRoute{}, false, false
	}
	reference, _, rooted := atom.Reference()
	if !rooted {
		if atom.RuntimeKinds().Contains(runtimekind.Function) {
			row, ok := calls.DispatchOpaqueRoute(key)
			return row, ok, ok
		}
		return calldomain.DispatchRoute{}, false, true
	}
	if require, scopedLoader := reference.ScopedLoader(); scopedLoader {
		boundaryRequire, hasRequire := calls.RequireOperation()
		loaderSeed, seedOK := candidate.LoaderSeedID()
		if !hasRequire || boundaryRequire != require || !seedOK {
			row, ok := calls.DispatchOpaqueRoute(key)
			return row, ok, ok
		}
		target, targetOK := calls.TargetForSeedID(loaderSeed)
		if !targetOK {
			row, ok := calls.DispatchOpaqueRoute(key)
			return row, ok, ok
		}
		row, ok := calls.DispatchTargetRoute(key, target)
		return row, ok, ok
	}
	if allocation, allocated := reference.AllocationKey(); allocated {
		module, _, allocationID, kind, _, originOK := heaps.AllocationOriginForKey(allocation)
		if originOK && kind == heapdomain.AllocationClosure {
			target, targetOK := calls.TargetForAllocation(module, allocationID)
			if targetOK {
				row, ok := calls.DispatchTargetRoute(key, target)
				return row, ok, ok
			}
		}
		if atom.RuntimeKinds().Contains(runtimekind.Function) {
			row, ok := calls.DispatchOpaqueRoute(key)
			return row, ok, ok
		}
		return calldomain.DispatchRoute{}, false, true
	}
	if seedID, callableSeed := reference.CallableID(); callableSeed {
		target, admitted := calls.TargetForSeedID(seedID)
		if !admitted {
			// CallableID is a cold owner decision. An absent target is an exact
			// denial, not runtime uncertainty.
			return calldomain.DispatchRoute{}, false, true
		}
		row, ok := calls.DispatchTargetRoute(key, target)
		return row, ok, ok
	}
	if seedID, endpoint := reference.EndpointID(); endpoint {
		target, admitted := calls.TargetForSeedID(seedID)
		if admitted {
			row, ok := calls.DispatchTargetRoute(key, target)
			return row, ok, ok
		}
		row, ok := calls.DispatchOpaqueRoute(key)
		return row, ok, ok
	}
	if atom.RuntimeKinds().Contains(runtimekind.Function) {
		row, ok := calls.DispatchOpaqueRoute(key)
		return row, ok, ok
	}
	return calldomain.DispatchRoute{}, false, true
}

func (plan *Plan) add(row calldomain.DispatchRoute) bool {
	if plan == nil || plan.count < 0 {
		return false
	}
	predicate, predicateOK := row.Predicate()
	if !predicateOK {
		return false
	}
	index := 0
	for index < plan.count {
		current, ok := plan.at(index)
		if !ok {
			return false
		}
		currentPredicate, ok := current.Predicate()
		if !ok {
			return false
		}
		if currentPredicate == predicate {
			return true
		}
		if currentPredicate > predicate {
			break
		}
		index++
	}
	if plan.count < inlineWidth {
		copy(plan.inline[index+1:plan.count+1], plan.inline[index:plan.count])
		plan.inline[index] = row
		plan.count++
		return true
	}
	plan.spill = append(plan.spill, calldomain.DispatchRoute{})
	if index < inlineWidth {
		displaced := plan.inline[inlineWidth-1]
		copy(plan.inline[index+1:], plan.inline[index:inlineWidth-1])
		plan.inline[index] = row
		copy(plan.spill[1:], plan.spill[:len(plan.spill)-1])
		plan.spill[0] = displaced
	} else {
		spillIndex := index - inlineWidth
		copy(plan.spill[spillIndex+1:], plan.spill[spillIndex:len(plan.spill)-1])
		plan.spill[spillIndex] = row
	}
	plan.count++
	return true
}

func (plan Plan) at(index int) (calldomain.DispatchRoute, bool) {
	if index < 0 || index >= plan.count {
		return calldomain.DispatchRoute{}, false
	}
	if index < inlineWidth {
		return plan.inline[index], true
	}
	index -= inlineWidth
	if index < 0 || index >= len(plan.spill) {
		return calldomain.DispatchRoute{}, false
	}
	return plan.spill[index], true
}

// Count and At are the direct relation accessors named by the schema source.
func Count(plan Plan) int { return plan.count }

func At(plan Plan, index int) (calldomain.DispatchRoute, bool) { return plan.at(index) }
