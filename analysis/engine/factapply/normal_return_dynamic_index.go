package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
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
	for _, table := range pathMembershipSourceTablesAt(out, resolver, ctx.Point, sourcePath) {
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

func applyNormalReturnDynamicIndexFacts(ctx normalReturnApplyContext, out state.State) state.State {
	edit := out.Edit(ctx.node.Registry)
	priorAllValueTables := make(map[keyspace.Key][]pathaddr.StateKey)
	if len(ctx.normalFacts.DynamicIndexFacts) != 0 {
		clearedContainers := make(map[keyspace.Key]struct{}, len(ctx.normalFacts.DynamicIndexFacts))
		for _, fact := range ctx.normalFacts.DynamicIndexFacts {
			tableKey, ok := ctx.keyspaceKey(fact.Table)
			if !ok {
				continue
			}
			if _, seen := clearedContainers[tableKey]; seen {
				continue
			}
			clearedContainers[tableKey] = struct{}{}
			priorAllValueTables[tableKey] = out.DynamicIndexAllValuesKeyMembershipTables(tableKey)
			out = out.ClearDynamicIndexValueKeyMembershipsForContainer(tableKey)
		}
	}
	dynamicChanged := false
	for _, fact := range ctx.normalFacts.DynamicIndexFacts {
		tableKey, ok := ctx.keyspaceKey(fact.Table)
		if !ok {
			continue
		}
		tablePath, ok := ctx.substitute(fact.Table)
		if !ok {
			continue
		}
		key := dynamicindex.Key{
			Table: tableKey,
			Site:  fact.Site,
		}
		valuePath, hasValuePath := ctx.substitute(fact.ValuePath)
		if edit.WriteDynamicIndexFact(key, fact.Value) {
			dynamicChanged = true
		}
		if hasValuePath {
			out = addDynamicIndexValueKeyMembershipsFromPath(ctx.node, ctx.resolver, out, valuePath, tableKey, key.Site)
		}
		out = writeHeapTableDynamicIndexFact(ctx.node, ctx.resolver, out, tablePath, key, fact.Value)
		if hasValuePath {
			out = addNormalReturnDynamicAllValueMembershipsFromPath(ctx, out, tablePath, tableKey, valuePath, fact.Value, priorAllValueTables[tableKey])
		}
	}
	if dynamicChanged {
		return edit.DoneOn(out)
	}
	return out
}

func addNormalReturnDynamicAllValueMembershipsFromPath(
	ctx normalReturnApplyContext,
	out state.State,
	tablePath pathdom.Path,
	tableKey keyspace.Key,
	valuePath pathdom.Path,
	value dynamicindex.Fact,
	priorTables []pathaddr.StateKey,
) state.State {
	if ctx.resolver == nil || valuePath.IsEmpty() || valuePath.Symbol == 0 {
		return out
	}
	sourceTables := pathMembershipSourceTablesAt(out, ctx.resolver, ctx.point, valuePath)
	if len(sourceTables) == 0 {
		return out
	}
	if dynamicIndexFactDefinitelyAbsent(ctx.node.Registry, value) {
		for _, table := range priorTables {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
		return out
	}
	for _, table := range priorTables {
		if stateKeyIn(sourceTables, table) {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
	}
	if _, freshAtCallEntry := ctx.freshDynamicIndexMutationTables[tableKey]; freshAtCallEntry {
		for _, table := range sourceTables {
			out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
		}
	}
	for _, table := range rootPathDynamicValueKeyMembershipTables(ctx.node.Registry, out, tablePath, tableKey) {
		out = out.AddDynamicIndexAllValuesKeyMembership(tableKey, table)
	}
	return out
}
