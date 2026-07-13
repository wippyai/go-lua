package program

import (
	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
)

// prepareStrictRelationOwners is the transaction gate for phase collapse.
// Every admitted owner is solved exactly once against the complete frozen
// relation summary universe.  Its concrete projection must equal the relation
// publication before any equation is omitted or Result is retained.
func prepareStrictRelationOwners(config body.Config, stats *Stats, activation *relationRunActivation, prepared preparedBodies, keys programKeys, forceReject bool) (*relationRunActivation, error) {
	if activation == nil {
		return nil, nil
	}
	pinned, exact := activation.snapshot.PinnedSummaries()
	if !exact || len(pinned) == 0 {
		if len(activation.snapshot.Entries()) != 0 && stats != nil {
			stats.RelationUnexpectedMisses++
			stats.RelationActivationFallbacks++
		}
		return nil, nil
	}
	reader := summary.NewSnapshot(config.Registry, pinned...)
	indexBase := summaryIndexBase(keys)
	results := make([]relationRetainedOwner, 0, len(pinned))
	for _, identity := range activation.snapshot.Entries() {
		runtime := activation.ownerRuntime(identity.Summary, identity.Prepared, nil, nil, summaryOwnerResolutionDigest(keys, identity.Summary))
		if !runtime.active || runtime.resolver == nil {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		tracked, observe := relationTrackedSummaryReader(config.Registry, reader, true, stats)
		missed := false
		ownerConfig := checkConfigWithSummaries(config, tracked, contextKeyFunc(keys, identity.Summary), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, identity.Summary), keys.metatableProof, runtime.resolver, observe, true, func() { missed = true }, stats)
		installRelationInputDigests(&ownerConfig, tracked, true)
		result, err := solvePreparedCountedWithTransfers(identity.Prepared, ownerConfig, prepassCounter(stats), nil, solveAttributionFor(stats, identity.Prepared, identity.Summary, SolvePhasePrepass, false))
		if err != nil {
			for _, retained := range results {
				retained.result.ReleaseTransient()
			}
			return nil, err
		}
		if missed {
			result.ReleaseTransient()
			return rejectStrictRelationOwners(stats, results), nil
		}
		if forceReject {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			result.ReleaseTransient()
			return rejectStrictRelationOwners(stats, results), nil
		}
		projected, err := summaryprojection.FromResultContext(config.Context, result)
		if err != nil {
			result.ReleaseTransient()
			for _, retained := range results {
				retained.result.ReleaseTransient()
			}
			return nil, err
		}
		want, ok := reader.Read(identity.Summary)
		if !ok || !summary.Equal(config.Registry, summary.Normalize(config.Registry, projected), want) {
			result.ReleaseTransient()
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		inputs := trackedSummaryReadDigests(config.Registry, tracked.(*trackingSummaryReader).deps)
		results = append(results, relationRetainedOwner{identity: identity, result: result, inputs: inputs})
	}
	activation.pinned = pinned
	activation.stats = stats
	for _, retained := range results {
		if !activation.retain(retained.identity, retained.result, retained.inputs) {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
	}
	return activation, nil
}

func rejectStrictRelationOwners(stats *Stats, retained []relationRetainedOwner) *relationRunActivation {
	for _, owner := range retained {
		owner.result.ReleaseTransient()
	}
	if stats != nil {
		stats.RelationActivationFallbacks++
	}
	return nil
}

func (a *relationRunActivation) omitsEquation(key summary.SummaryKey, prepared *body.Static) bool {
	_, ok := a.retainedResult(key, prepared)
	return ok
}
