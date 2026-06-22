package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
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
	targetKey := factPathKeyAt(resolver, ctx.Point, fact.TargetPath())
	if targetKey == "" {
		return out
	}
	source := fact.Source()
	value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, out))
	if !ok {
		return out
	}
	ks := resolver.KeySpace()
	out = out.WritePathStaticMember(ks, targetKey, value)
	if canonical, ok := fieldCanonicalPathKey(ks, targetKey); ok {
		out = out.WritePathStaticMember(ks, canonical, value)
	}
	out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, fact.TargetPath(), source)
	return addPathEqualityProofFromDynamicIndexSource(ctx, resolver, facts, sources, read, in, out, fact.TargetPath(), source)
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
	tableStateKey := factPathKeyAt(resolver, ctx.Point, fact.TablePath())
	if tableStateKey == "" {
		return out
	}
	tableKey, ok := resolver.KeySpace().FromStateKey(tableStateKey)
	if !ok {
		return out
	}
	key := dynamicindex.Key{
		Table: tableKey,
		Site:  dynamicindex.SiteForPoint(int(ctx.Point)),
	}
	value := dynamicIndexFact(ctx, sources, read, in, out, fact)
	out = out.WriteDynamicIndexFact(ctx.Registry, key, value)
	out = addKnownDynamicIndexWriteEquality(ctx, resolver, facts, out, fact, value)
	return writeHeapTableDynamicIndexFact(ctx, resolver, out, fact.TablePath(), key, value)
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
	return addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, fact.TablePath().IndexStr(name), fact.Source())
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
		if keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithSamePointCallSource(ctx.Point, keySource, read, current)); ok {
			config.KeyValue = keyValue
			config.HasKeyValue = true
		}
	}
	if readValue {
		source := fact.Source()
		if value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, current)); ok {
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
	ctx transfer.EdgeContext,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	out state.State,
	proof factflow.BranchPathEvidence,
) state.State {
	if proof.Kind() == factflow.BranchPathEvidenceFrozenTable {
		return applyFrozenTableFact(ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.Path())
	}
	stateProof, ok := branchPathEvidenceAt(resolver, ctx.Edge.From, proof)
	if !ok {
		return out
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
	pathKey := factPathKeyAt(resolver, point, proof.Path())
	if pathKey == "" {
		return pathevidence.BranchProof{}, false
	}
	path, ok := resolver.KeySpace().FromStateKey(pathKey)
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
	otherKey, ok := branchPathEvidenceOtherPathKeyAt(resolver, point, proof)
	if !ok {
		return keyspace.Key{}, false
	}
	return resolver.KeySpace().FromStateKey(otherKey)
}

func branchPathEvidenceOtherPathKeyAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	proof factflow.BranchPathEvidence,
) (pathdom.PathKey, bool) {
	otherPath, ok := proof.OtherPath()
	if !ok {
		return "", false
	}
	otherKey := factPathKeyAt(resolver, point, otherPath)
	if otherKey == "" {
		return "", false
	}
	return otherKey, true
}

func factPathKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) pathdom.PathKey {
	key, ok := factStateKeyAt(resolver, point, path)
	if !ok {
		return ""
	}
	return key.PathKey()
}

func factStateKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) (pathaddr.StateKey, bool) {
	if resolver == nil {
		return "", false
	}
	return resolver.StateKeyAt(point, path)
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
	sourcePath, ok := facts.ExpressionPath(source.ExprRef)
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
	keyValue, ok := sources.ValueOfSource(ctx.Point, dyn.KeySource(), in, readWithSamePointCallSource(ctx.Point, dyn.KeySource(), read, out))
	if !ok {
		return out
	}
	name, ok := staticStringKey(ctx.Registry, keyValue)
	if !ok {
		return out
	}
	return addPathEqualityProofAt(resolver, ctx.Point, out, targetPath, dyn.TablePath().IndexStr(name))
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
	targetKey := factPathKeyAt(resolver, point, targetPath)
	sourceKey := factPathKeyAt(resolver, point, sourcePath)
	if targetKey == "" || sourceKey == "" || targetKey == sourceKey {
		return out
	}
	targetKeyStruct, ok := resolver.KeySpace().FromStateKey(targetKey)
	if !ok {
		return out
	}
	sourceKeyStruct, ok := resolver.KeySpace().FromStateKey(sourceKey)
	if !ok {
		return out
	}
	out = out.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  targetKeyStruct,
		Other: sourceKeyStruct,
	})
	return out.CanonicalizeTypestateResources(resolver.KeySpace())
}
