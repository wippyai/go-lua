package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
)

// reduce maps one exact Value callee to Call's direct selected-target
// relation. It is the only cross-domain completeness judgment; Call core
// receives only owner-fenced Target capabilities at the final seam.
func reduce(bound site, callee valuedomain.Value) (calldomain.Value, bool) {
	algebra := bound.algebraOwner()
	coordinate, coordinateOK := bound.valueCoordinate()
	key, keyOK := bound.callKey()
	values := bound.valueSchema()
	if algebra == nil || !keyOK || !coordinateOK || values == nil || !values.AdmitsCoordinate(coordinate, callee) {
		return calldomain.Value{}, false
	}
	if callee.IsTop() {
		return algebra.Top(), true
	}
	if callee.IsBottom() {
		return algebra.DispatchValue(key, nil, false)
	}

	targets := make([]calldomain.Target, 0, 2)
	unknown := false
	if !values.VisitAtoms(callee, func(atom valuedomain.Atom) bool {
		capability, known, callable := dispatchAtom(bound, atom)
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
	return algebra.DispatchValue(key, targets, unknown)
}

// dispatchAtom maps one exact Value atom. known means one admitted target;
// callable means an unresolved callable alternative requiring Call's opaque
// arm. Known non-functions contribute no target and no opaque alternative.
func dispatchAtom(bound site, atom valuedomain.Atom) (capability calldomain.Target, known, callable bool) {
	algebra := bound.algebraOwner()
	values := bound.valueSchema()
	if algebra == nil || values == nil || !values.OwnsAtom(atom) {
		return calldomain.Target{}, false, true
	}
	reference, _, rooted := atom.Reference()
	if !rooted {
		return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
	}
	if require, scopedLoader := reference.ScopedLoader(); scopedLoader {
		boundaryRequire, hasRequire := algebra.RequireOperation()
		key, keyOK := bound.callKey()
		if !hasRequire || boundaryRequire != require || !keyOK || !key.IsApplication() {
			return calldomain.Target{}, false, true
		}
		seedID := bound.requireSeedID
		capability, admitted := algebra.TargetForSeedID(seedID)
		admitted = admitted && seedID.Available()
		if !admitted {
			return calldomain.Target{}, false, true
		}
		return capability, true, false
	}

	if key, allocation := reference.AllocationKey(); allocation {
		if !values.OwnsHeapSchema(bound.heaps) {
			return calldomain.Target{}, false, true
		}
		receipt, programAllocation := key.AllocationReceipt()
		if programAllocation && receipt.Available() && receipt.Kind() == heapdomain.AllocationClosure {
			capability, admitted := algebra.TargetForAllocation(receipt.Module(), receipt.AllocationID())
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
