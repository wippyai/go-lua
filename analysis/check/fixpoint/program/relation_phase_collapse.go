package program

import (
	"slices"

	"github.com/wippyai/go-lua/analysis/check/body"
	summaryprojection "github.com/wippyai/go-lua/analysis/check/fixpoint/program/internal/projectsummary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/transformer"
)

// prepareStrictRelationOwners is the transaction gate for phase collapse.
// Every admitted lexical base is solved exactly once in dependency order.
// Zero-boundary projections must equal their frozen relation; parameterized
// bases become concrete pins for exact dependency lineage. Every specialized
// context is then solved and compared with its frozen relation publication.
// Nothing is omitted or retained until the whole transaction succeeds.
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
	baseOrder, exact := strictRelationBaseOrder(activation.snapshot)
	if !exact {
		if stats != nil {
			stats.RelationUnexpectedMisses++
		}
		return rejectStrictRelationOwners(stats, nil), nil
	}
	readAuthority, exact := newStrictRelationReadAuthority(activation.snapshot, baseOrder)
	if !exact {
		if stats != nil {
			stats.RelationUnexpectedMisses++
		}
		return rejectStrictRelationOwners(stats, nil), nil
	}
	published := make(map[summary.SummaryKey]summary.Summary, len(pinned)+len(baseOrder))
	for _, entry := range pinned {
		published[entry.Key] = summary.Normalize(config.Registry, entry.Summary)
	}
	indexBase := summaryIndexBase(keys)
	results := make([]relationRetainedOwner, 0, len(pinned)+len(baseOrder))
	for rank, identity := range baseOrder {
		relation, found := activation.snapshot.Lookup(identity)
		if !found {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		origin, hasOrigin := strictRelationBaseOrigin(prepared, keys, identity)
		if relation.Shape() != (transformer.Shape{}) && !hasOrigin {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		reader := strictRelationPublishedReader(published)
		tracked, observe := relationTrackedSummaryReader(config.Registry, reader, true, stats)
		ownerConfig := checkConfigWithSummaries(config, tracked, contextKeyFunc(keys, identity.Summary), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, identity.Summary), keys.metatableProof, nil, observe, true, nil, stats)
		installRelationInputDigests(&ownerConfig, tracked, true)
		if relation.Shape() != (transformer.Shape{}) {
			ownerConfig = keyedFunctionMaterializeConfig(identity.Prepared, ownerConfig, keys, tracked, origin)
		}
		result, err := solvePreparedCountedWithTransfers(identity.Prepared, ownerConfig, prepassCounter(stats), nil, solveAttributionFor(stats, identity.Prepared, identity.Summary, SolvePhasePrepass, false))
		if err != nil {
			if result != nil {
				result.ReleaseTransient()
			}
			for _, retained := range results {
				retained.result.ReleaseTransient()
			}
			return nil, err
		}
		trackedReader := tracked.(*trackingSummaryReader)
		if !readAuthority.permits(trackedReader.deps, rank, true) {
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
		normalized := summary.Normalize(config.Registry, projected)
		if relation.Shape() == (transformer.Shape{}) {
			want, ok := published[identity.Summary]
			if !ok || !summary.Equal(config.Registry, normalized, want) {
				result.ReleaseTransient()
				if stats != nil {
					stats.RelationUnexpectedMisses++
				}
				return rejectStrictRelationOwners(stats, results), nil
			}
		} else if _, duplicate := published[identity.Summary]; duplicate {
			result.ReleaseTransient()
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		published[identity.Summary] = normalized
		inputs := trackedSummaryInputs(config.Context, config.Registry, trackedReader.deps)
		results = append(results, relationRetainedOwner{identity: identity, result: result, inputs: inputs})
	}
	pinned = strictRelationPublishedEntries(published)
	reader := summary.NewSnapshot(config.Registry, pinned...)
	contexts := append([]relationContextSummary(nil), activation.snapshot.contexts...)
	slices.SortFunc(contexts, func(a, b relationContextSummary) int {
		aRank, aOK := readAuthority.base[a.base.Summary]
		bRank, bOK := readAuthority.base[b.base.Summary]
		if !aOK || !bOK {
			if aOK {
				return -1
			}
			if bOK {
				return 1
			}
		} else if aRank != bRank {
			if aRank < bRank {
				return -1
			}
			return 1
		}
		if a.context.Less(b.context) {
			return -1
		}
		if b.context.Less(a.context) {
			return 1
		}
		return 0
	})
	for _, contextual := range contexts {
		rank, ranked := readAuthority.base[contextual.base.Summary]
		if !ranked {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		origin, ok := keys.contexts.contextByKey(contextual.context)
		exactEntry := ok && origin != nil && origin.relationContextEntry == contextual.certificate
		validatedFrame := ok && origin != nil && contextual.validatedFrame != nil && contextual.validatedFrame.matchesFullContext(config.Registry, origin, contextual.base, keys.contexts.discoveryGeneration)
		if !ok || origin == nil || (!exactEntry && !validatedFrame) || origin.funcExpr == nil || contextual.base.Prepared == nil || contextual.base.Prepared != prepared.function(origin.funcExpr) {
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		tracked, observe := relationTrackedSummaryReader(config.Registry, reader, true, stats)
		ownerConfig := checkConfigWithSummaries(config, tracked, contextKeyFunc(keys, contextual.context), directKeyFunc(keys), summaryIndexForOwner(indexBase, keys, contextual.context), keys.metatableProof, nil, observe, true, nil, stats)
		installRelationInputDigests(&ownerConfig, tracked, true)
		ownerConfig = keyedFunctionMaterializeConfig(contextual.base.Prepared, ownerConfig, keys, tracked, *origin)
		result, err := solvePreparedCountedWithTransfers(contextual.base.Prepared, ownerConfig, prepassCounter(stats), nil, solveAttributionFor(stats, contextual.base.Prepared, contextual.context, SolvePhasePrepass, true))
		if err != nil {
			if result != nil {
				result.ReleaseTransient()
			}
			for _, retained := range results {
				retained.result.ReleaseTransient()
			}
			return nil, err
		}
		trackedReader := tracked.(*trackingSummaryReader)
		if !readAuthority.permits(trackedReader.deps, rank, false) || forceReject {
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
		want, ok := reader.Read(contextual.context)
		normalizedProjected := summary.Normalize(config.Registry, projected)
		if !ok || !summary.Equal(config.Registry, normalizedProjected, want) {
			result.ReleaseTransient()
			if stats != nil {
				stats.RelationUnexpectedMisses++
			}
			return rejectStrictRelationOwners(stats, results), nil
		}
		identity := contextual.base
		identity.Summary = contextual.context
		inputs := trackedSummaryInputs(config.Context, config.Registry, trackedReader.deps)
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

// strictRelationBaseOrder returns the generation-local producer DAG in
// dependency-first order. The planner already rejects recursive SCCs; checking
// again at publication makes that assumption transactional rather than ambient.
func strictRelationBaseOrder(snapshot relationRunSnapshot) ([]relationCellIdentity, bool) {
	entries := snapshot.Entries()
	byCell := make(map[transformer.CellRef]relationCellIdentity, len(entries))
	for _, identity := range entries {
		if identity.Generation != snapshot.generation {
			return nil, false
		}
		byCell[identity.Cell] = identity
	}
	state := make(map[transformer.CellRef]uint8, len(entries))
	ordered := make([]relationCellIdentity, 0, len(entries))
	var visit func(relationCellIdentity) bool
	visit = func(identity relationCellIdentity) bool {
		switch state[identity.Cell] {
		case 1:
			return false
		case 2:
			return true
		}
		state[identity.Cell] = 1
		owner := relationConsumerIdentity{
			Summary: identity.Summary, BodyDigest: identity.BodyDigest,
			Prepared: identity.Prepared, Generation: identity.Generation,
		}
		direct, exact := snapshot.DirectCalls(owner)
		if !exact {
			return false
		}
		dependencies := direct.Cells()
		slices.SortFunc(dependencies, func(a, b transformer.CellRef) int {
			if a.Function != b.Function {
				if a.Function < b.Function {
					return -1
				}
				return 1
			}
			if a.Slot < b.Slot {
				return -1
			}
			if a.Slot > b.Slot {
				return 1
			}
			return 0
		})
		for _, cell := range dependencies {
			dependency, present := byCell[cell]
			if !present || !visit(dependency) {
				return false
			}
		}
		state[identity.Cell] = 2
		ordered = append(ordered, identity)
		return true
	}
	for _, identity := range entries {
		if !visit(identity) {
			return nil, false
		}
	}
	return ordered, true
}

func strictRelationBaseOrigin(prepared preparedBodies, keys programKeys, identity relationCellIdentity) (keyedFunction, bool) {
	for _, origin := range keys.functions {
		if origin.funcExpr != nil && prepared.function(origin.funcExpr) == identity.Prepared {
			origin.key = identity.Summary
			return origin, true
		}
	}
	return keyedFunction{}, false
}

func strictRelationPublishedEntries(published map[summary.SummaryKey]summary.Summary) []summary.EntrySummary {
	out := make([]summary.EntrySummary, 0, len(published))
	for key, sum := range published {
		out = append(out, summary.EntrySummary{Key: key, Summary: sum})
	}
	slices.SortFunc(out, func(a, b summary.EntrySummary) int {
		if a.Key.Less(b.Key) {
			return -1
		}
		if b.Key.Less(a.Key) {
			return 1
		}
		return 0
	})
	return out
}

// strictRelationPublishedReader is a transaction-private evolving universe.
// It mutates only between synchronous base solves; every stored payload is
// normalized and owned by the transaction. The final publication is rebuilt as
// an immutable Snapshot after the dependency-ordered base phase completes.
type strictRelationPublishedReader map[summary.SummaryKey]summary.Summary

func (r strictRelationPublishedReader) Read(key summary.SummaryKey) (summary.Summary, bool) {
	sum, ok := r[key]
	if !ok {
		return summary.Summary{}, false
	}
	return sum.Clone(), true
}

func (r strictRelationPublishedReader) ReadOwnedNormalized(key summary.SummaryKey) (summary.Summary, bool) {
	sum, ok := r[key]
	return sum, ok
}

type strictRelationReadAuthority struct {
	base     map[summary.SummaryKey]int
	contexts map[summary.SummaryKey]int
	zeroPins map[summary.SummaryKey]struct{}
}

func newStrictRelationReadAuthority(snapshot relationRunSnapshot, order []relationCellIdentity) (strictRelationReadAuthority, bool) {
	out := strictRelationReadAuthority{
		base: make(map[summary.SummaryKey]int, len(order)), contexts: make(map[summary.SummaryKey]int, len(snapshot.contexts)),
		zeroPins: make(map[summary.SummaryKey]struct{}),
	}
	for rank, identity := range order {
		if _, duplicate := out.base[identity.Summary]; duplicate {
			return strictRelationReadAuthority{}, false
		}
		relation, exact := snapshot.Lookup(identity)
		if !exact {
			return strictRelationReadAuthority{}, false
		}
		out.base[identity.Summary] = rank
		if relation.Shape() == (transformer.Shape{}) {
			out.zeroPins[identity.Summary] = struct{}{}
		}
	}
	for _, contextual := range snapshot.contexts {
		rank, exact := out.base[contextual.base.Summary]
		if !exact || contextual.context.Ref.IsZero() {
			return strictRelationReadAuthority{}, false
		}
		if _, duplicate := out.contexts[contextual.context]; duplicate {
			return strictRelationReadAuthority{}, false
		}
		out.contexts[contextual.context] = rank
	}
	return out, true
}

// permits proves that every summary consumed by one concrete validation solve
// belongs to an earlier producer in the same acyclic transaction. Merely being
// present is insufficient because all provisional context pins are visible
// before validation. Zero-boundary relation pins are independent exact
// theorems and may seed the base phase at any rank; contexts must still depend
// strictly downward.
func (a strictRelationReadAuthority) permits(reads map[summary.SummaryKey]trackedSummaryRead, rank int, basePhase bool) bool {
	for key, read := range reads {
		if !read.present {
			return false
		}
		if dependencyRank, known := a.base[key]; known {
			if dependencyRank < rank {
				continue
			}
			if basePhase {
				if _, exactZeroPin := a.zeroPins[key]; exactZeroPin {
					continue
				}
			}
			return false
		}
		if dependencyRank, known := a.contexts[key]; known && dependencyRank < rank {
			continue
		}
		return false
	}
	return true
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
