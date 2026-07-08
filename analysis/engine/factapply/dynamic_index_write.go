package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyDynamicIndexWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
) state.State {
	tablePath := fact.TablePathRef()
	tableKey, ok := dynamicIndexWriteContainerKeyAt(resolver, ctx.Point, tablePath)
	if !ok {
		return out
	}
	key := dynamicindex.Key{
		Table: tableKey,
		Site:  dynamicindex.SiteForPoint(int(ctx.Point)),
	}
	value := dynamicIndexFact(ctx, sources, read, in, out, fact)
	allValueTables := out.DynamicIndexAllValuesKeyMembershipTables(tableKey)
	pendingRestores := pendingDynamicAllValueRestoresFromPrimaryDelete(ctx, resolver, facts, out, fact, value)
	out = out.ClearDynamicIndexValueKeyMembershipsForContainer(tableKey)
	out = clearKeyMembershipsForMaybeAbsentDynamicWrite(ctx, resolver, out, fact, value)
	for _, restore := range pendingRestores {
		out = out.AddPendingDynamicAllValueRestore(restore.Container, restore.Table, restore.Key)
	}
	out = out.WriteDynamicIndexFact(ctx.Registry, key, value)
	out = addPathKeyMembershipFromDynamicWrite(ctx, resolver, facts, out, fact, value)
	out = addDynamicIndexValueKeyMembershipsFromWrite(ctx, resolver, facts, in, out, fact, value, tableKey, key.Site)
	out = preserveDynamicIndexAllValueKeyMemberships(ctx, resolver, facts, in, out, fact, value, tableKey, allValueTables)
	out = restorePendingDynamicAllValuesFromReverseDelete(ctx, resolver, facts, out, fact, value, tableKey)
	out = addKnownDynamicIndexWriteEquality(ctx, resolver, facts, out, fact, value)
	out = addKnownDynamicIndexWriteStaticMember(ctx, resolver, out, fact, value)
	out = applyStoredDynamicIndexPlacement(ctx, resolver, out, tablePath, value.Value)
	return writeHeapTableDynamicIndexFact(ctx, resolver, out, tablePath, key, value)
}

func pendingDynamicAllValueRestoresFromPrimaryDelete(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) []state.PendingDynamicAllValueRestore {
	if resolver == nil || dynamicIndexFactDefinitelyPresent(ctx.Registry, value) {
		return nil
	}
	keyStateKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact)
	if !ok {
		return nil
	}
	origins := in.DynamicIndexReadOriginsForValue(keyStateKey)
	if len(origins) == 0 {
		return nil
	}
	var out []state.PendingDynamicAllValueRestore
	tablePath := fact.TablePathRef()
	forEachDynamicWriteTableStateKeyAt(resolver, ctx.Point, tablePath, func(tableStateKey pathaddr.StateKey) bool {
		for _, container := range in.DynamicIndexAllValuesKeyMembershipContainers(tableStateKey) {
			for _, origin := range origins {
				if origin.Container == container {
					out = append(out, state.PendingDynamicAllValueRestore{
						Container: container,
						Table:     tableStateKey,
						Key:       origin.Key,
					})
				}
			}
		}
		return true
	})
	return out
}

func clearKeyMembershipsForMaybeAbsentDynamicWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) state.State {
	if dynamicIndexFactDefinitelyPresent(ctx.Registry, value) {
		return out
	}
	tablePath := fact.TablePathRef()
	forEachDynamicWriteTableStateKeyAt(resolver, ctx.Point, tablePath, func(tableStateKey pathaddr.StateKey) bool {
		out = out.ClearKeyMembershipsForPath(tableStateKey)
		return true
	})
	if resolver != nil && tablePath.Symbol != 0 {
		out = out.ClearKeyMembershipsForTableSymbol(resolver.KeySpace(), tablePath.Symbol)
	}
	return out
}

func forEachDynamicWriteTableStateKeyAt(resolver *visibility.Resolver, point cfg.Point, tablePath pathdom.Path, fn func(pathaddr.StateKey) bool) bool {
	if resolver == nil || tablePath.IsEmpty() || tablePath.Symbol == 0 {
		return true
	}
	return visibility.AddressAt(resolver, point, tablePath).ForEachStateKey(fn,
		visibility.StateKeyVisible,
		visibility.StateKeyRootOrVisible,
		visibility.StateKeyStructural,
	)
}

func dynamicIndexWriteKeyStateKeyAt(resolver *visibility.Resolver, point cfg.Point, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathaddr.StateKey, bool) {
	keyPath, ok := dynamicIndexWriteKeyPath(resolver, facts, fact)
	if !ok {
		return "", false
	}
	return visibility.AddressAt(resolver, point, keyPath).VisibleStateKey()
}

func dynamicIndexWriteKeyPath(resolver *visibility.Resolver, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if keyPath, ok := fact.KeyPathRef(); ok && keyPath.Symbol != 0 {
		return keyPath, true
	}
	sourcePath, ok := callSourcePath(facts, resolver, fact.KeySource())
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return sourcePath, true
}

func dynamicIndexWriteContainerKeyAt(resolver *visibility.Resolver, point cfg.Point, tablePath pathdom.Path) (keyspace.Key, bool) {
	return visibility.AddressAt(resolver, point, tablePath).RootOrVisibleKeyspaceKey()
}

func restorePendingDynamicAllValuesFromReverseDelete(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
	container keyspace.Key,
) state.State {
	if resolver == nil || !dynamicIndexFactDefinitelyAbsent(ctx.Registry, value) {
		return out
	}
	keyStateKey, ok := dynamicIndexWriteKeyStateKeyAt(resolver, ctx.Point, facts, fact)
	if !ok {
		return out
	}
	keys := append([]pathaddr.StateKey{keyStateKey}, out.EquivalentStateKeys(resolver.KeySpace(), keyStateKey)...)
	for _, key := range keys {
		for _, restore := range out.PendingDynamicAllValueRestores(container, key) {
			out = out.AddDynamicIndexAllValuesKeyMembership(restore.Container, restore.Table)
			out = out.ClearPendingDynamicAllValueRestore(restore)
		}
	}
	return out
}

func preserveDynamicIndexAllValueKeyMemberships(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
	container keyspace.Key,
	tables []pathaddr.StateKey,
) state.State {
	if len(tables) == 0 {
		return out
	}
	if dynamicIndexFactDefinitelyAbsent(ctx.Registry, value) {
		for _, table := range tables {
			out = out.AddDynamicIndexAllValuesKeyMembership(container, table)
		}
		return out
	}
	if resolver == nil {
		return out
	}
	sourcePath, ok := dynamicIndexWriteSourcePath(resolver, facts, fact)
	if !ok {
		return out
	}
	sourceTables := pathMembershipSourceTablesAt(in, resolver, ctx.Point, sourcePath)
	for _, table := range tables {
		if stateKeyIn(sourceTables, table) {
			out = out.AddDynamicIndexAllValuesKeyMembership(container, table)
		}
	}
	return out
}

func dynamicIndexFactDefinitelyAbsent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if presence.Equal(product.PresenceOf(fact.Value), presence.Absent()) {
		return true
	}
	return typevalue.HasOnlyNilType(reg, fact.Value)
}

func dynamicIndexFactDefinitelyPresent(reg *axis.Registry, fact dynamicindex.Fact) bool {
	if reg == nil || product.Equal(reg, fact.Value, product.Bottom(reg)) {
		return false
	}
	if typevalue.HasOnlyNilType(reg, fact.Value) {
		return false
	}
	return presence.Equal(product.PresenceOf(fact.Value), presence.Present())
}

func stateKeyIn(keys []pathaddr.StateKey, want pathaddr.StateKey) bool {
	for _, key := range keys {
		if key == want {
			return true
		}
	}
	return false
}

func addPathKeyMembershipFromDynamicWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) state.State {
	if resolver == nil || !dynamicIndexFactDefinitelyPresent(ctx.Registry, value) {
		return out
	}
	tableKey, ok := visibility.RootOrVisibleStateKeyAt(resolver, ctx.Point, fact.TablePathRef())
	if !ok {
		return out
	}
	keyPath, ok := dynamicIndexWriteKeyPath(resolver, facts, fact)
	if !ok || keyPath.IsEmpty() || keyPath.Symbol == 0 {
		return out
	}
	keyStateKey, ok := visibility.AddressAt(resolver, ctx.Point, keyPath).VisibleStateKey()
	if !ok {
		return out
	}
	return out.AddPathKeyMembership(keyStateKey, tableKey)
}

func addDynamicIndexValueKeyMembershipsFromWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
	container keyspace.Key,
	site dynamicindex.Site,
) state.State {
	if resolver == nil || !dynamicIndexFactDefinitelyPresent(ctx.Registry, value) {
		return out
	}
	sourcePath, ok := dynamicIndexWriteSourcePath(resolver, facts, fact)
	if !ok {
		return out
	}
	for _, table := range pathMembershipSourceTablesAt(in, resolver, ctx.Point, sourcePath) {
		out = out.AddDynamicIndexValueKeyMembership(container, site, table)
	}
	return out
}

func dynamicIndexWriteSourcePath(resolver *visibility.Resolver, facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if valuePath, ok := fact.ValuePathRef(); ok && !valuePath.IsEmpty() && valuePath.Symbol != 0 {
		return valuePath, true
	}
	sourcePath, ok := callSourcePath(facts, resolver, fact.Source())
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return pathdom.Path{}, false
	}
	return sourcePath, true
}

func addKnownDynamicIndexWriteEquality(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) state.State {
	name, ok := staticStringKey(ctx.Registry, value.KeyValue)
	if !ok {
		return out
	}
	return addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, fact.TablePathRef().IndexStr(name), fact.Source())
}

func addKnownDynamicIndexWriteStaticMember(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	fact factflow.DynamicIndexWrite,
	value dynamicindex.Fact,
) state.State {
	if resolver == nil || product.Equal(ctx.Registry, value.Value, product.Bottom(ctx.Registry)) {
		return out
	}
	name, ok := staticStringKey(ctx.Registry, value.KeyValue)
	if !ok {
		return out
	}
	targetPath := fact.TablePathRef().IndexStr(name)
	targetKey := factPathKeyAt(resolver, ctx.Point, targetPath)
	if targetKey == "" {
		return out
	}
	ks := resolver.KeySpace()
	localKey, ok := ks.FromPathKey(targetKey)
	if !ok {
		return out
	}
	edit := out.Edit(ctx.Registry)
	edit.WriteLocalPathStaticMember(localKey, value.Value)
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		edit.WriteLocalPathStaticMember(canonical, value.Value)
	}
	return edit.Done()
}

func writeHeapTableDynamicIndexFact(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	tablePath pathdom.Path,
	key dynamicindex.Key,
	value dynamicindex.Fact,
) state.State {
	table, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, tablePath, nil)
	if !ok {
		return out
	}
	id, ok := product.Get(ctx.Registry, table.value, identity.Key).ID()
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(ctx.Registry, id)
	if heapidentity.ObjectDomain(ctx.Registry).Equal(object, heapidentity.BottomObject(ctx.Registry)) {
		return out
	}
	dynamic := object.DynamicIndexFacts()
	if dynamic == nil {
		dynamic = make(map[dynamicindex.Key]dynamicindex.Fact, 1)
	}
	if existing, ok := dynamic[key]; ok {
		dynamic[key] = dynamicindex.Domain(ctx.Registry).Join(existing, value)
	} else {
		dynamic[key] = value
	}
	return out.WriteHeapTableObject(ctx.Registry, id, heapidentity.NewTableObject(heapidentity.TableObjectConfig{
		Root:              object.Root(),
		StaticMembers:     object.StaticMembers(),
		DynamicIndexFacts: dynamic,
	}))
}

func applyStoredDynamicIndexPlacement(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	tablePath pathdom.Path,
	value product.Value,
) state.State {
	if resolver == nil || tablePath.IsEmpty() {
		return out
	}
	table, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, tablePath, nil)
	if !ok {
		return out
	}
	tableID, ok := product.Get(ctx.Registry, table.value, identity.Key).ID()
	if !ok {
		return out
	}
	ownerPlacement := out.ReadPlacement(tableID)
	switch ownerPlacement {
	case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
		return markReachableHeapValuePlacement(ctx.Registry, out, value, ownerPlacement, map[identity.ID]struct{}{})
	default:
		return out
	}
}

func dynamicIndexFact(
	ctx transfer.NodeContext,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	current state.State,
	fact factflow.DynamicIndexWrite,
) dynamicindex.Fact {
	config := dynamicindex.FactConfig{Admission: fact.Admission()}
	readKey, readValue := dynamicIndexReadback(fact.ReadbackIntent())
	if readKey {
		keySource := fact.KeySource()
		if keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithCurrentPointState(ctx.Point, read, current)); ok {
			config.KeyValue = keyValue
			config.HasKeyValue = true
		}
	}
	if readValue {
		source := fact.Source()
		if value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, current)); ok {
			config.Value = value
			config.HasValue = true
		}
	}
	return dynamicindex.NewFact(ctx.Registry, config)
}

func dynamicIndexReadback(intent factflow.DynamicIndexReadbackIntent) (readKey bool, readValue bool) {
	switch intent {
	case factflow.DynamicIndexReadbackKey:
		return true, false
	case factflow.DynamicIndexReadbackValue:
		return false, true
	case factflow.DynamicIndexReadbackKeyAndValue:
		return true, true
	default:
		return false, false
	}
}
