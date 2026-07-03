package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
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
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func applyPathStaticMemberWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.PathStaticMemberWrite,
) state.State {
	targetPath := fact.TargetPathRef()
	targetKey := factPathKeyAt(resolver, ctx.Point, targetPath)
	if targetKey == "" {
		return out
	}
	source := fact.Source()
	value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return out
	}
	ks := resolver.KeySpace()
	localKey, ok := ks.FromPathKey(targetKey)
	if !ok {
		return out
	}
	edit := out.Edit(ctx.Registry)
	edit.WriteLocalPathStaticMember(localKey, value)
	if canonical, ok := ks.FieldCanonical(localKey); ok {
		edit.WriteLocalPathStaticMember(canonical, value)
	}
	out = edit.Done()
	out = writeHeapTableStaticMember(ctx, resolver, out, targetPath, value)
	out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, targetPath, source)
	return addPathEqualityProofFromDynamicIndexSource(ctx, resolver, facts, sources, read, in, out, targetPath, source)
}

func writeHeapTableStaticMember(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
	value product.Value,
) state.State {
	if resolver == nil || len(targetPath.Segments) == 0 {
		return out
	}
	ownerPath := targetPath.Clone()
	suffix := ownerPath.Segments[len(ownerPath.Segments)-1:]
	ownerPath.Segments = ownerPath.Segments[:len(ownerPath.Segments)-1]
	owner, ok := resolvePathValueAt(ctx.Registry, resolver, ctx.Point, out, ownerPath, nil)
	if !ok {
		return out
	}
	id, ok := product.Get(ctx.Registry, owner.value, identity.Key).ID()
	if !ok {
		return out
	}
	object := out.ReadHeapTableObject(ctx.Registry, id)
	if heapidentity.ObjectDomain(ctx.Registry).Equal(object, heapidentity.BottomObject(ctx.Registry)) {
		return out
	}
	object, ok = object.WithStaticMember(resolver.KeySpace(), suffix, value)
	if !ok {
		return out
	}
	return out.WriteHeapTableObject(ctx.Registry, id, object)
}

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
	keyPath, ok := dynamicIndexWriteKeyPath(facts, fact)
	if !ok {
		return "", false
	}
	return visibility.AddressAt(resolver, point, keyPath).VisibleStateKey()
}

func dynamicIndexWriteKeyPath(facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if keyPath, ok := fact.KeyPathRef(); ok && keyPath.Symbol != 0 {
		return keyPath, true
	}
	source := fact.KeySource()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return pathdom.Path{}, false
	}
	sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
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
	sourcePath, ok := dynamicIndexWriteSourcePath(facts, fact)
	if !ok {
		return out
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, sourcePath).VisibleStateKey()
	if !ok {
		return out
	}
	sourceTables := in.PathKeyMembershipTables(sourceKey)
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
	keySource := fact.KeySource()
	if keySource.Kind != factflow.ValueSourceExpression || !keySource.HasExpr {
		return out
	}
	keyPath, ok := facts.ExpressionPathRef(keySource.ExprRef)
	if !ok || keyPath.IsEmpty() || keyPath.Symbol == 0 {
		return out
	}
	keyStateKey, ok := factStateKeyAt(resolver, ctx.Point, keyPath)
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
	sourcePath, ok := dynamicIndexWriteSourcePath(facts, fact)
	if !ok {
		return out
	}
	sourceKey, ok := visibility.AddressAt(resolver, ctx.Point, sourcePath).VisibleStateKey()
	if !ok {
		return out
	}
	for _, table := range in.PathKeyMembershipTables(sourceKey) {
		out = out.AddDynamicIndexValueKeyMembership(container, site, table)
	}
	return out
}

func dynamicIndexWriteSourcePath(facts factflow.Facts, fact factflow.DynamicIndexWrite) (pathdom.Path, bool) {
	if valuePath, ok := fact.ValuePathRef(); ok && !valuePath.IsEmpty() && valuePath.Symbol != 0 {
		return valuePath, true
	}
	source := fact.Source()
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return pathdom.Path{}, false
	}
	sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
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

func applyBranchPathEvidence(
	typeValues *typevalue.Cache,
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	proof factflow.BranchPathEvidence,
) state.State {
	if proof.Kind() == factflow.BranchPathEvidenceFrozenTable {
		return applyFrozenTableFact(ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.PathRef())
	}
	stateProof, ok := branchPathEvidenceAt(resolver, ctx.Edge.From, proof)
	if !ok {
		return out
	}
	if proof.Kind() == factflow.BranchPathEvidenceEqual {
		if other, ok := proof.OtherPathRef(); ok {
			if selected, applied := applyChannelSelectCaseEquality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.PathRef(), other); applied {
				out = selected
			} else {
				out = applyPathEqualityAtCached(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.PathRef(), other)
			}
			if stateIsBottom(ctx.Registry, out) {
				return out
			}
		}
	}
	out = out.AddBranchProof(stateProof)
	if stateProof.Kind == pathevidence.BranchProofPathEqual {
		ks := resolver.KeySpace()
		out = closeCongruenceAcrossEquality(ctx.Registry, ks, out, ks.Format(stateProof.Path), ks.Format(stateProof.Other))
		out = out.CanonicalizeTypestateResources(ks)
	}
	return out
}

// closeCongruenceAcrossEquality propagates existing path refinements across a
// newly proven equality aKey == bKey, so a fact recorded on one alias (or its
// member, under reference equality) refines the other regardless of the order
// the equality and the refinement were applied. It runs once per proven
// equality, off the hot read path, and is idempotent (it meets into existing
// facts).
func closeCongruenceAcrossEquality(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, aKey, bKey pathdom.PathKey) state.State {
	if aKey == "" || bKey == "" || aKey == bKey {
		return out
	}
	snap := out.PathRefinementsSnapshot(ks)
	if snap.Bottom || len(snap.Refinements) == 0 {
		return out
	}
	// Member (field/index) congruence is sound only under reference equality. Lua
	// == is reference equality for tables that declare no __eq metamethod; the
	// engine does not model __eq, so a proven table equality is reference-safe, and
	// non-table values have no members to make congruent. If __eq is ever modeled,
	// exclude types that declare it here.
	memberSafe := pathValueMayBeTable(reg, ks, out, aKey) && pathValueMayBeTable(reg, ks, out, bKey)
	for key, value := range snap.Refinements {
		out = propagateRefinementAcrossEquality(reg, ks, out, key, value, aKey, bKey, memberSafe)
		out = propagateRefinementAcrossEquality(reg, ks, out, key, value, bKey, aKey, memberSafe)
	}
	return out
}

func propagateRefinementAcrossEquality(reg *axis.Registry, ks *keyspace.KeySpace, out state.State, key pathdom.PathKey, value product.Value, fromKey, toKey pathdom.PathKey, memberSafe bool) state.State {
	rebased, ok := pathaddr.RebasePathKey(key, fromKey, toKey)
	if !ok || rebased == "" || rebased == key {
		return out
	}
	// key == fromKey is the equal root itself (presence transfer, sound even under
	// __eq); a deeper key is a member, gated on reference equality.
	if key != fromKey && !memberSafe {
		return out
	}
	current := out.ReadPathKey(reg, ks, rebased)
	merged := value
	if !product.Equal(reg, current, product.Bottom(reg)) {
		merged = product.Meet(reg, current, value)
		if product.Equal(reg, merged, product.Bottom(reg)) {
			return out
		}
	}
	return out.WritePathKey(reg, ks, rebased, merged)
}

func pathValueMayBeTable(reg *axis.Registry, ks *keyspace.KeySpace, in state.State, key pathdom.PathKey) bool {
	value := in.ReadPathKey(reg, ks, key)
	hasValue := !product.Equal(reg, value, product.Bottom(reg))
	return sourcevalue.RuntimeMayBeTable(reg, value, hasValue)
}

func branchPathEvidenceAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	proof factflow.BranchPathEvidence,
) (pathevidence.BranchProof, bool) {
	path, ok := factKeyspaceKeyAt(resolver, point, proof.PathRef())
	if !ok {
		return pathevidence.BranchProof{}, false
	}
	switch proof.Kind() {
	case factflow.BranchPathEvidencePresence:
		value, ok := proof.Presence()
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     path,
			Presence: value,
		}, true
	case factflow.BranchPathEvidenceEqual:
		other, ok := branchPathEvidenceOtherKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  path,
			Other: other,
		}, true
	case factflow.BranchPathEvidenceNotEqual:
		other, ok := branchPathEvidenceOtherKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  path,
			Other: other,
		}, true
	case factflow.BranchPathEvidenceIndexInRange:
		other, ok := branchPathEvidenceOtherKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofIndexInRange,
			Path:  path,
			Other: other,
		}, true
	default:
		return pathevidence.BranchProof{}, false
	}
}

func branchPathEvidenceOtherKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	proof factflow.BranchPathEvidence,
) (keyspace.Key, bool) {
	otherPath, ok := proof.OtherPathRef()
	if !ok {
		return keyspace.Key{}, false
	}
	return factKeyspaceKeyAt(resolver, point, otherPath)
}

func factPathKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) pathdom.PathKey {
	key, ok := visibility.AddressAt(resolver, point, path).VisiblePathKey()
	if !ok {
		return ""
	}
	return key
}

func factStateKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) (pathaddr.StateKey, bool) {
	return visibility.AddressAt(resolver, point, path).VisibleStateKey()
}

func factKeyspaceKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) (keyspace.Key, bool) {
	return visibility.AddressAt(resolver, point, path).VisibleKeyspaceKey()
}

func addPathEqualityProofFromSource(
	resolver *visibility.Resolver,
	facts factflow.Facts,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if resolver == nil || targetPath.Symbol == 0 || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return out
	}
	sourcePath, ok := facts.ExpressionPathRef(source.ExprRef)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	// A covariant record exposure of this source (the object exposed through a wider
	// mutable view at the same point) must not leave a target == source equality:
	// the narrow per-field facts would meet back onto the widened source through
	// reference-equality member congruence, undoing the exposure widen. The eager
	// source widen carries the sound widened type instead.
	if covariantExposureSuppressesPathProof(facts, point, source) {
		return out
	}
	return addPathEqualityProofAt(resolver, point, out, targetPath, sourcePath)
}

func addPathEqualityProofFromDynamicIndexSource(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	facts factflow.Facts,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	targetPath pathdom.Path,
	source factflow.ValueSource,
) state.State {
	if source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return out
	}
	dyn, ok := facts.DynamicIndexExpression(source.ExprRef)
	if !ok {
		return out
	}
	keySource := dyn.KeySource()
	keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithCurrentPointState(ctx.Point, read, out))
	if !ok {
		return out
	}
	name, ok := staticStringKey(ctx.Registry, keyValue)
	if !ok {
		return out
	}
	return addPathEqualityProofAt(resolver, ctx.Point, out, targetPath, dyn.TablePathRef().IndexStr(name))
}

func staticStringKey(reg *axis.Registry, value product.Value) (string, bool) {
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return "", false
	}
	lit, ok := t.(*typ.Literal)
	if !ok || lit.Base != kind.String {
		return "", false
	}
	name, ok := lit.Value.(string)
	return name, ok
}

func addPathEqualityProofAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	sourcePath pathdom.Path,
) state.State {
	targetStateKey, targetStateOK := factStateKeyAt(resolver, point, targetPath)
	sourceStateKey, sourceStateOK := factStateKeyAt(resolver, point, sourcePath)
	if !targetStateOK || !sourceStateOK || targetStateKey == sourceStateKey {
		return out
	}
	targetKey, ok := visibility.KeyspaceKeyFromStateKey(resolver, targetStateKey)
	if !ok {
		return out
	}
	sourceKey, ok := visibility.KeyspaceKeyFromStateKey(resolver, sourceStateKey)
	if !ok {
		return out
	}
	out = out.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  targetKey,
		Other: sourceKey,
	})
	return out.CanonicalizeTypestateResources(resolver.KeySpace())
}
