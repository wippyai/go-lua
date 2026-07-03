package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
)

func normalReturnDynamicIndexMutationTables(facts []callboundary.DynamicIndexFact) []pathdom.Path {
	if len(facts) == 0 {
		return nil
	}
	out := make([]pathdom.Path, 0, len(facts))
	for _, fact := range facts {
		if fact.Table.IsEmpty() {
			continue
		}
		out = append(out, fact.Table)
	}
	return out
}

func freshDynamicIndexMutationTablesAtCallEntry(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	boundaryPaths callboundary.PathBindings,
	in state.State,
	facts []callboundary.DynamicIndexFact,
) map[keyspace.Key]struct{} {
	if resolver == nil || len(facts) == 0 {
		return nil
	}
	counts := make(map[keyspace.Key]int, len(facts))
	paths := make(map[keyspace.Key]pathdom.Path, len(facts))
	for _, fact := range facts {
		tableKey, ok := callOutcomeKeyspaceKeyAt(resolver, ctx.Point, boundaryPaths, fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := boundaryPaths.Substitute(fact.Table)
		if !ok {
			continue
		}
		counts[tableKey]++
		paths[tableKey] = tablePath
	}
	out := make(map[keyspace.Key]struct{})
	for tableKey, count := range counts {
		if count != 1 {
			continue
		}
		if rootPathHasFreshEmptyTable(ctx.Registry, in, paths[tableKey]) {
			out[tableKey] = struct{}{}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func addDynamicIndexValueKeyMembershipsFromPath(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	sourcePath pathdom.Path,
	container keyspace.Key,
	site dynamicindex.Site,
) state.State {
	if resolver == nil || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, sourcePath).VisibleStateKey()
	if !ok {
		return out
	}
	for _, table := range out.PathKeyMembershipTables(sourceKey) {
		out = out.AddDynamicIndexValueKeyMembership(container, site, table)
	}
	return out
}

func normalReturnPathMatchesAny(target pathdom.Path, candidates []pathdom.Path) bool {
	for _, candidate := range candidates {
		if target.Equal(candidate) {
			return true
		}
	}
	return false
}
