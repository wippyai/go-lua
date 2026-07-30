package state

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
)

func projectHeapBoundary(ctx *boundaryProjectContext, source State, out *State) bool {
	out.heapTableIdentity, _ = projectHeapBoundaryFactor(ctx, source.heapTableIdentity)
	return true
}
func projectHeapBoundaryFactor(ctx *boundaryProjectContext, source heapTableIdentityLane) (heapTableIdentityLane, bool) {
	if source.top {
		return source, true
	}
	values := projectFiniteMap(source.values, func(term identity.Term, _ heapidentity.TableObject) bool {
		return ctx.closure.ContainsIdentityTerm(term)
	})
	for id, object := range values {
		values[id] = object.MapValues(ctx.reg, func(value product.Value) product.Value {
			return product.ProjectBoundary(ctx.reg, value)
		})
	}
	return heapTableIdentityLaneFromMap(heapTermMapDomain(ctx.reg), values), true
}
func rebaseHeapBoundary(ctx *boundaryRebaseContext, source State, out *State) bool {
	var ok bool
	out.heapTableIdentity, ok = rebaseHeapBoundaryFactor(ctx, source.heapTableIdentity)
	return ok
}
func rebaseHeapBoundaryFactor(ctx *boundaryRebaseContext, source heapTableIdentityLane) (heapTableIdentityLane, bool) {
	if source.top {
		return source, true
	}
	values := make(map[identity.Term]heapidentity.TableObject, len(source.values))
	for term, object := range source.values {
		image, ok := identityImage(ctx, term)
		if !ok {
			return heapTableIdentityLane{}, false
		}
		if image.IsBottom() {
			ctx.relationBottom = true
			return heapTableIdentityLane{}, true
		}
		if image.IsTop() {
			return heapTableIdentityLane{top: true}, true
		}
		nextTerm, exact := image.Term()
		if !exact {
			return heapTableIdentityLane{}, false
		}
		object, ok = object.Rekey(ctx.fromKeys, ctx.toKeys)
		if !ok {
			return heapTableIdentityLane{}, false
		}
		valid := true
		object = object.MapValues(ctx.reg, func(value product.Value) product.Value {
			next, good := rebaseBoundaryProduct(ctx, value)
			valid = valid && good
			return next
		})
		if !valid {
			return heapTableIdentityLane{}, false
		}
		if existing, collision := values[nextTerm]; collision {
			// A recurrent lexical allocation site maps both its template and
			// prior abstract site identity to one finite site. Join both heap
			// payloads; overwriting either side would lose semantics.
			object = heapidentity.ObjectDomain(ctx.reg).Join(existing, object)
		}
		values[nextTerm] = object
	}
	return heapTableIdentityLaneFromMap(heapTermMapDomain(ctx.reg), values), true
}
func applyHeapBoundaryLane(ctx *boundaryApplyContext, destination, fragment heapTableIdentityLane) (heapTableIdentityLane, bool) {
	if destination.top || fragment.top {
		return heapTableIdentityLane{top: true}, true
	}
	objectDomain := heapidentity.ObjectDomain(ctx.reg)
	values := applyFiniteMapEqual(
		destination.values,
		fragment.values,
		func(term identity.Term, _ heapidentity.TableObject) bool {
			return ctx.closure.ContainsIdentityTerm(term)
		},
		objectDomain.Equal,
	)
	return heapTableIdentityLaneFromMap(heapTermMapDomain(ctx.reg), values), true
}

// applyHeapBoundary remains a test-facing adapter over the single typed lane
// implementation; production boundary execution never reconstructs State.
func applyHeapBoundary(ctx *boundaryApplyContext, destination, fragment State, out *State) bool {
	lane, ok := applyHeapBoundaryLane(ctx, destination.heapTableIdentity, fragment.heapTableIdentity)
	out.heapTableIdentity = lane
	return ok
}
func equalHeapBoundary(reg *axis.Registry, a, b State) bool {
	d := heapTermMapDomain(reg)
	return d.Equal(a.heapTableIdentity.asMap(d), b.heapTableIdentity.asMap(d))
}
