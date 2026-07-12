package program

import (
	"context"
	"errors"

	"github.com/wippyai/go-lua/analysis/check/body"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

func freezeRelationActivation(ctx context.Context, stats *Stats, catalog relationRunCatalog) (*relationRunActivation, error) {
	recordRelationActivationCensus(stats, catalog)
	frozen, err := catalog.Freeze(ctx)
	if err == nil {
		return newRelationRunActivation(frozen), nil
	}
	// Cancellation belongs to the whole program transaction and must remain
	// observable. A relation-specific preparation/certification rejection only
	// declines the optimization: legacy solving is still complete and sound.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil, err
	}
	if stats != nil {
		stats.RelationActivationFallbacks++
	}
	return nil, nil
}

// relationRunActivation is the single immutable authority installed for one
// program run.  Resolver routing, cache fencing and retained-state ownership
// are deliberately derived together so no active owner can observe a mixture
// of relation and legacy generations.
type relationRunActivation struct {
	snapshot relationRunSnapshot
	policy   relationOwnerCachePolicy
}

type relationOwnerRuntime struct {
	active         bool
	resolver       relationResolverFactory
	cache          *SummarySolveCache
	retained       *retainedSummaryApplicationOwner
	resolution     uint64
	contextSummary *summary.Summary
}

func (a *relationRunActivation) contextOwnerRuntime(
	origin keyedFunction,
	prepared *body.Static,
	legacyCache *SummarySolveCache,
	retained *retainedSummaryApplicationRun,
	legacyResolution uint64,
) relationOwnerRuntime {
	if a == nil || prepared == nil || origin.relationContextEntry == nil {
		return a.ownerRuntime(origin.key, prepared, legacyCache, retained, legacyResolution)
	}
	sum, ok := a.snapshot.ContextSummary(origin.key, origin.relationContextEntry, prepared.IdentityDigest())
	if !ok {
		return a.ownerRuntime(origin.key, prepared, legacyCache, retained, legacyResolution)
	}
	// A certified context equation is itself fulfilled by the frozen relation.
	// It owns no legacy cache/retained publication even when its body has no
	// relation calls of its own.
	return relationOwnerRuntime{active: true, resolution: legacyResolution, contextSummary: &sum}
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

func recordRelationActivationCensus(stats *Stats, catalog relationRunCatalog) {
	if stats == nil {
		return
	}
	stats.RelationProducersEligible += len(catalog.entries)
	stats.RelationContextsSpecialized += len(catalog.contexts)
	for _, owner := range catalog.consumers.entries {
		if owner.active {
			stats.RelationOwnersActive++
		}
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
