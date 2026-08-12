package dispatch

import (
	calldomain "github.com/wippyai/go-lua/analysis/domain/call"
	heapdomain "github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/runtimekind"
	valuedomain "github.com/wippyai/go-lua/analysis/domain/value"
	linkboundary "github.com/wippyai/go-lua/program/link/boundary"
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
		boundaryRequire, hasRequire := bound.boundary.RequireOperation()
		key, keyOK := bound.callKey()
		application, applicationOK := key.Application()
		shard, _, callOK := bound.link.Project().Applications().Call(application)
		if !hasRequire || boundaryRequire != require || !keyOK || !applicationOK || !callOK {
			return calldomain.Target{}, false, true
		}
		seed, seedOK := bound.boundary.Seeds().ScopedLoader(shard)
		if !seedOK {
			return calldomain.Target{}, false, true
		}
		capability, admitted := algebra.TargetForSeed(seed)
		if !admitted {
			return calldomain.Target{}, false, true
		}
		return capability, true, false
	}

	if key, allocation := reference.AllocationKey(); allocation {
		if !values.OwnsHeapSchema(bound.heaps) {
			return calldomain.Target{}, false, true
		}
		shard, function, kind, programAllocation := key.ProgramAllocation()
		if programAllocation && kind == heapdomain.AllocationClosure {
			capability, admitted := algebra.TargetForFunction(shard, function)
			if !admitted {
				return calldomain.Target{}, false, true
			}
			return capability, true, false
		}
		return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
	}

	if seed, callableSeed := reference.Callable(); callableSeed {
		disposition, _, _, classified := bound.boundary.Seeds().CallableDisposition(seed)
		if !classified {
			return calldomain.Target{}, false, true
		}
		switch disposition {
		case linkboundary.CallableDeniedTarget:
			return calldomain.Target{}, false, false
		case linkboundary.CallableAdmittedOperation:
			capability, admitted := algebra.TargetForSeed(seed)
			if !admitted {
				return calldomain.Target{}, false, true
			}
			return capability, true, false
		default:
			return calldomain.Target{}, false, true
		}
	}

	if endpoint, endpointReference := reference.Endpoint(); endpointReference {
		seed, seedOK := bound.boundary.Endpoints().Seed(endpoint)
		if !seedOK {
			return calldomain.Target{}, false, true
		}
		capability, admitted := algebra.TargetForSeed(seed)
		if !admitted {
			return calldomain.Target{}, false, true
		}
		return capability, true, false
	}

	return calldomain.Target{}, false, atom.RuntimeKinds().Contains(runtimekind.Function)
}
