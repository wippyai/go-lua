package state

import (
	"context"
	"fmt"

	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func visitProductIdentity(reg *axis.Registry, value product.Value, visit func(identity.Term) bool) bool {
	if term, exact := product.Get(reg, value, identity.Key).Term(); exact {
		return visit(term)
	}
	return true
}

// ValueHasIdentitySupport reports whether a product value contributes an
// exact identity term to the registered substitution inventory. Values which
// carry no such term are independent product components for identity-only
// quotienting; Top and Bottom therefore report false.
func (d ProductDomain) ValueHasIdentitySupport(value product.Value) (bool, error) {
	if !d.Valid() || !product.BelongsToRegistry(d.reg, value) {
		return false, fmt.Errorf("%w: product value identity support", ErrInvalidLaneFactor)
	}
	found := false
	visitProductIdentity(d.reg, value, func(identity.Term) bool {
		found = true
		return false
	})
	return found, nil
}

func visitValuesLaneIdentities(reg *axis.Registry, source valueLane, visit func(identity.Term) bool) bool {
	if source.top {
		return true
	}
	for _, value := range source.symbols {
		if !visitProductIdentity(reg, value, visit) {
			return false
		}
	}
	for _, value := range source.returns {
		if !visitProductIdentity(reg, value, visit) {
			return false
		}
	}
	return true
}

func visitPathEvidenceLaneIdentities(reg *axis.Registry, source pathevidence.Lane, visit func(identity.Term) bool) bool {
	keepGoing := true
	source.ForEachPathRefinement(func(_ keyspace.Key, value product.Value) bool {
		keepGoing = visitProductIdentity(reg, value, visit)
		return keepGoing
	})
	if !keepGoing {
		return false
	}
	source.ForEachPathStaticMember(func(_ keyspace.Key, value product.Value) bool {
		keepGoing = visitProductIdentity(reg, value, visit)
		return keepGoing
	})
	if !keepGoing {
		return false
	}
	source.ForEachPathPresenceImplication(func(value pathevidence.PathPresenceImplication) bool {
		if value.HasTriggerValue {
			if !visitProductIdentity(reg, value.TriggerValue, visit) {
				keepGoing = false
				return false
			}
		}
		if value.HasTargetValue {
			if !visitProductIdentity(reg, value.TargetValue, visit) {
				keepGoing = false
				return false
			}
		}
		return true
	})
	return keepGoing
}

func visitDynamicIndexLaneIdentities(reg *axis.Registry, source dynamicIndexLane, visit func(identity.Term) bool) bool {
	if source.top {
		return true
	}
	for _, fact := range source.values {
		if !visitProductIdentity(reg, fact.KeyValue, visit) || !visitProductIdentity(reg, fact.Value, visit) {
			return false
		}
	}
	return true
}

func visitHeapTableIdentityLaneIdentities(reg *axis.Registry, source heapTableIdentityLane, visit func(identity.Term) bool) bool {
	if source.top {
		return true
	}
	for id, object := range source.values {
		if !visit(id) || !visitProductIdentity(reg, object.Root(), visit) {
			return false
		}
		keepGoing := true
		object.VisitStaticMembers(func(_ keyspace.Key, value product.Value) bool {
			keepGoing = visitProductIdentity(reg, value, visit)
			return keepGoing
		})
		if !keepGoing {
			return false
		}
		object.VisitDynamicIndexFacts(func(_ dynamicindex.Key, fact dynamicindex.Fact) bool {
			keepGoing = visitProductIdentity(reg, fact.KeyValue, visit) && visitProductIdentity(reg, fact.Value, visit)
			return keepGoing
		})
		if !keepGoing {
			return false
		}
	}
	return true
}

func visitFrozenTablesLaneIdentities(_ *axis.Registry, source frozenTableLane, visit func(identity.Term) bool) bool {
	for term := range source.values {
		if !visit(term) {
			return false
		}
	}
	return true
}

func visitEffectDeltasLaneIdentities(reg *axis.Registry, source effectDeltaLane, visit func(identity.Term) bool) bool {
	if source.top {
		return true
	}
	for _, delta := range source.values {
		if !visitProductIdentity(reg, delta.Before, visit) || !visitProductIdentity(reg, delta.After, visit) {
			return false
		}
	}
	return true
}

func visitChannelSelectLaneIdentities(reg *axis.Registry, source channelselectfact.Lane, visit func(identity.Term) bool) bool {
	return source.ForEachFact(func(fact channelselectfact.Fact) bool {
		if fact.HasPayload && !visitProductIdentity(reg, fact.Payload, visit) {
			return false
		}
		return true
	})
}

func visitPlacementLaneIdentities(_ *axis.Registry, source placementLane, visit func(identity.Term) bool) bool {
	for term := range source.values {
		if !visit(term) {
			return false
		}
	}
	return true
}

// stateContainsAllocationTemplate is the support-map-free common-path query.
// It traverses the same mandatory policy as complete support collection and
// stops at the first exact lexical template.
func stateContainsAllocationTemplate(ctx context.Context, reg *axis.Registry, source State) (bool, error) {
	complete, err := visitStateIdentities(ctx, reg, source, continueWithoutAllocationTemplate)
	return !complete, err
}

func continueWithoutAllocationTemplate(term identity.Term) bool {
	_, allocation := term.Allocation()
	return !allocation
}

func visitStateIdentities(
	ctx context.Context,
	reg *axis.Registry,
	source State,
	visit func(identity.Term) bool,
) (bool, error) {
	if reg == nil {
		return false, fmt.Errorf("state: identity inventory requires registry")
	}
	if visit == nil {
		return false, fmt.Errorf("state: identity inventory requires visitor")
	}
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return false, err
		}
	}
	for _, spec := range defaultLaneCatalog.specs {
		if !source.laneMask.allows(spec.bit) {
			continue
		}
		switch spec.identitySupport.kind {
		case laneIdentitiesIndependent:
		case laneIdentitiesEnumerated:
			// This zero-allocation adapter invokes the same typed visitor registered
			// for factors. It never reconstructs a State from a factor.
			if !spec.identitySupport.visitState(reg, source, visit) {
				return false, nil
			}
		default:
			return false, fmt.Errorf("state: lane %q has no identity-support policy", spec.id)
		}
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return false, err
			}
		}
	}
	return true, nil
}
