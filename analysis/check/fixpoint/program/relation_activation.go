package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

// relationRunActivation is the single immutable authority installed for one
// program run.  Resolver routing, cache fencing and retained-state ownership
// are deliberately derived together so no active owner can observe a mixture
// of relation and legacy generations.
type relationRunActivation struct {
	snapshot relationRunSnapshot
	policy   relationOwnerCachePolicy
}

type relationOwnerRuntime struct {
	active     bool
	resolver   relationResolverFactory
	cache      *SummarySolveCache
	retained   *retainedSummaryApplicationOwner
	resolution uint64
}

type relationMaterializedRuntime struct {
	resolver   relationResolverFactory
	cache      *materializedSolveCache
	resolution uint64
	active     bool
}

func newRelationRunActivation(snapshot relationRunSnapshot) *relationRunActivation {
	if snapshot.generation == nil {
		return nil
	}
	return &relationRunActivation{
		snapshot: snapshot,
		policy:   newRelationOwnerCachePolicy(snapshot.consumers),
	}
}

func (a *relationRunActivation) ownerIdentity(key summary.SummaryKey, prepared *body.Static) (relationConsumerIdentity, bool) {
	if a == nil || prepared == nil || a.snapshot.generation == nil {
		return relationConsumerIdentity{}, false
	}
	owner := relationConsumerIdentity{
		Summary: key, BodyDigest: prepared.IdentityDigest(), Prepared: prepared,
		Generation: a.snapshot.generation,
	}
	if !a.policy.active(owner) {
		return relationConsumerIdentity{}, false
	}
	return owner, true
}

func (a *relationRunActivation) ownerRuntime(
	key summary.SummaryKey,
	prepared *body.Static,
	legacyCache *SummarySolveCache,
	retained *retainedSummaryApplicationRun,
	legacyResolution uint64,
) relationOwnerRuntime {
	out := relationOwnerRuntime{cache: legacyCache, resolution: legacyResolution}
	owner, ok := a.ownerIdentity(key, prepared)
	if !ok {
		if retained != nil {
			out.retained = retained.newOwner(key)
		}
		return out
	}
	resolver, ok := a.snapshot.inactiveRelationResolverFactory(owner)
	if !ok {
		if retained != nil {
			out.retained = retained.newOwner(key)
		}
		return out
	}
	// All four choices flip as one transaction.
	out.active = true
	out.resolver = resolver
	out.cache = a.policy.summaryCache(owner, legacyCache)
	out.resolution = a.policy.resolutionDigest(owner, legacyResolution)
	return out
}

func (a *relationRunActivation) materializedOwnerRuntime(
	key summary.SummaryKey,
	prepared *body.Static,
	legacyCache *materializedSolveCache,
	legacyResolution uint64,
) relationMaterializedRuntime {
	out := relationMaterializedRuntime{cache: legacyCache, resolution: legacyResolution}
	owner, ok := a.ownerIdentity(key, prepared)
	if !ok {
		return out
	}
	resolver, ok := a.snapshot.inactiveRelationResolverFactory(owner)
	if !ok {
		return out
	}
	out.active = true
	out.resolver = resolver
	out.cache = legacyCache.withoutRetainedHandoff()
	out.resolution = a.policy.resolutionDigest(owner, legacyResolution)
	return out
}
