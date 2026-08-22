package dispatch

import (
	calldomain "github.com/wippyai/go-lua/domain/call"
	heapdomain "github.com/wippyai/go-lua/domain/heap"
	"github.com/wippyai/go-lua/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/domain/value"
)

// reduce maps one exact Value callee to Call's direct selected-target
// relation. It is the only cross-domain completeness judgment; Call core
// receives only owner-fenced Target capabilities at the final seam.
func reduce(rule *HotRule, mounted calldomain.MountedCall, bound dispatchRow, callee valuedomain.Value) (calldomain.Value, bool) {
	if rule == nil || !rule.valid() {
		return calldomain.Value{}, false
	}
	algebra := rule.calls.Algebra()
	values := rule.values.Schema()
	if !bound.key.Valid() || !bound.coordinate.Valid() || !values.AdmitsCoordinate(bound.coordinate, callee) {
		return calldomain.Value{}, false
	}
	if callee.IsTop() {
		return algebra.Top(), true
	}
	if callee.IsBottom() {
		return algebra.DispatchValue(bound.key, nil, false)
	}
	targets := make([]calldomain.Target, 0, 2)
	unknown := false
	if !values.VisitAtoms(callee, func(atom valuedomain.Atom) bool {
		capability, known, callable := dispatchAtom(rule, mounted, bound, atom)
		if known {
			targets = append(targets, capability)
		}
		if callable {
			unknown = true
		}
		return true
	}) {
		return calldomain.Value{}, false
	}
	return algebra.DispatchValue(bound.key, targets, unknown)
}

// dispatchAtom maps one exact Value atom. known means one admitted target;
// callable means an unresolved callable alternative requiring Call's opaque
// arm. Known non-functions contribute no target and no opaque alternative.
func dispatchAtom(rule *HotRule, mounted calldomain.MountedCall, bound dispatchRow, atom valuedomain.Atom) (capability calldomain.Target, known, callable bool) {
	algebra := rule.calls.Algebra()
	values := rule.values.Schema()
	if !values.OwnsAtom(atom) {
		return calldomain.Target{}, false, true
	}
	reference, _, rooted := atom.Reference()
	if !rooted {
		return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
	}
	if require, scopedLoader := reference.ScopedLoader(); scopedLoader {
		boundaryRequire, hasRequire := algebra.RequireOperation()
		if !hasRequire || boundaryRequire != require || !bound.key.IsApplication() {
			return calldomain.Target{}, false, true
		}
		_, _, _, _, seedID, identityOK := algebra.MountedCallIdentity(mounted)
		capability, admitted := algebra.TargetForSeedID(seedID)
		admitted = admitted && identityOK && seedID.Available()
		if !admitted {
			return calldomain.Target{}, false, true
		}
		return capability, true, false
	}

	if key, allocation := reference.AllocationKey(); allocation {
		if !values.OwnsHeapSchema(rule.heaps) {
			return calldomain.Target{}, false, true
		}
		module, _, allocationID, kind, _, programAllocation := rule.heaps.AllocationOriginForKey(key)
		if programAllocation && kind == heapdomain.AllocationClosure {
			capability, admitted := algebra.TargetForAllocation(module, allocationID)
			if !admitted {
				return calldomain.Target{}, false, true
			}
			return capability, true, false
		}
		return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
	}

	if seedID, callableSeed := reference.CallableID(); callableSeed {
		capability, admitted := algebra.TargetForSeedID(seedID)
		if admitted {
			return capability, true, false
		}
		// Value issues CallableID only for an exact callable seed admitted or
		// explicitly denied during the cold Link seal. Absence from Call's
		// admitted target rows is therefore the exact denied disposition.
		return calldomain.Target{}, false, false
	}

	if seedID, endpointReference := reference.EndpointID(); endpointReference {
		capability, admitted := algebra.TargetForSeedID(seedID)
		if !admitted {
			return calldomain.Target{}, false, true
		}
		return capability, true, false
	}

	return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
}
