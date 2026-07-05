package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/keyspace"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
	out = applyPathStaticMemberWriteContainerPresence(ctx, resolver, out, targetPath)
	out = writeHeapTableStaticMember(ctx, resolver, out, targetPath, value)
	out = addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, targetPath, source)
	return addPathEqualityProofFromDynamicIndexSource(ctx, resolver, facts, sources, read, in, out, targetPath, source)
}

func applyPathStaticMemberWriteContainerPresence(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	targetPath pathdom.Path,
) state.State {
	if targetPath.Symbol == 0 || len(targetPath.Segments) == 0 {
		return out
	}
	present := product.NewWithPresence(ctx.Registry, product.ShapeTop, presence.Present())
	for parent := targetPath.Parent(); !parent.IsEmpty(); parent = parent.Parent() {
		out = applyValueRefinementAt(ctx.Registry, resolver, nil, ctx.Point, out, parent, factflow.NewValueConstraint(present))
	}
	return out
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
	ks := resolver.KeySpace()
	if stateProof.Kind == pathevidence.BranchProofPathEqual || out.HasBranchProofKind(pathevidence.BranchProofPathEqual) {
		out = closeBranchProofsAcrossKnownEqualities(ks, out)
	}
	if stateProof.Kind == pathevidence.BranchProofPathEqual {
		out = closeCongruenceAcrossEquality(ctx.Registry, ks, out, ks.Format(stateProof.Path), ks.Format(stateProof.Other))
		out = out.CanonicalizeTypestateResources(ks)
	}
	return activatePathPresenceImplications(ctx.Registry, resolver, ctx.Edge.From, out)
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

func closeBranchProofsAcrossEquality(ks *keyspace.KeySpace, out state.State, aKey, bKey keyspace.Key) state.State {
	if ks == nil || aKey == bKey {
		return out
	}
	snap := out.BranchProofsSnapshot(ks)
	if snap.Bottom || snap.Top || len(snap.Proofs) == 0 {
		return out
	}
	for _, proof := range snap.Proofs {
		out = mirrorBranchProofAcrossEquality(ks, out, proof, aKey, bKey)
		out = mirrorBranchProofAcrossEquality(ks, out, proof, bKey, aKey)
	}
	return out
}

func closeBranchProofsAcrossKnownEqualities(ks *keyspace.KeySpace, out state.State) state.State {
	if ks == nil {
		return out
	}
	snap := out.BranchProofsSnapshot(ks)
	if snap.Bottom || snap.Top || len(snap.Proofs) == 0 {
		return out
	}
	for _, proof := range snap.Proofs {
		if proof.Kind != pathevidence.BranchProofPathEqual {
			continue
		}
		out = closeBranchProofsAcrossEquality(ks, out, proof.Path, proof.Other)
	}
	return out
}

func mirrorBranchProofAcrossEquality(ks *keyspace.KeySpace, out state.State, proof pathevidence.BranchProof, fromKey, toKey keyspace.Key) state.State {
	rebasedPath, ok := rebaseBranchProofKey(ks, proof.Path, fromKey, toKey)
	if !ok {
		return out
	}
	mirrored := proof
	mirrored.Path = rebasedPath
	switch proof.Kind {
	case pathevidence.BranchProofPathPresence:
		return out.AddBranchProof(mirrored)
	case pathevidence.BranchProofIndexInRange:
		if proof.Other != (keyspace.Key{}) {
			if rebasedOther, otherOK := rebaseBranchProofKey(ks, proof.Other, fromKey, toKey); otherOK {
				mirrored.Other = rebasedOther
			}
		}
		return out.AddBranchProof(mirrored)
	default:
		return out
	}
}

func rebaseBranchProofKey(ks *keyspace.KeySpace, proofKey, fromKey, toKey keyspace.Key) (keyspace.Key, bool) {
	if !ks.HasPrefix(proofKey, fromKey) {
		return keyspace.Key{}, false
	}
	if ks.HasStrictPrefix(toKey, fromKey) && ks.HasPrefix(proofKey, toKey) {
		return keyspace.Key{}, false
	}
	rebased, ok := ks.Rebase(proofKey, fromKey, toKey)
	if !ok || !ks.HasPrefix(rebased, toKey) || rebased == proofKey {
		return keyspace.Key{}, false
	}
	return rebased, true
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
	if resolver == nil || targetPath.Symbol == 0 {
		return out
	}
	sourcePath, ok := sourcePathFromValueSource(resolver, facts, source)
	if !ok || sourcePath.IsEmpty() || sourcePath.Symbol == 0 {
		return out
	}
	// A covariant record exposure of this source (the object exposed through a wider
	// mutable view at the same point) must not leave a target == source equality:
	// the narrow per-field facts would meet back onto the widened source through
	// reference-equality member congruence, undoing the exposure widen. The eager
	// source widen carries the sound widened type instead.
	if covariantExposureSuppressesPathProof(facts, resolver, point, source) {
		return out
	}
	return addPathEqualityProofAt(resolver, point, out, targetPath, sourcePath)
}

func sourcePathFromValueSource(
	resolver *visibility.Resolver,
	facts factflow.Facts,
	source factflow.ValueSource,
) (pathdom.Path, bool) {
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		return facts.ExpressionPathRef(source.ExprRef)
	}
	if source.Kind != factflow.ValueSourcePath || source.PathKey == "" || resolver == nil || resolver.KeySpace() == nil {
		return pathdom.Path{}, false
	}
	key, ok := resolver.KeySpace().FromStateKey(source.PathKey)
	if !ok || key.Sym == 0 {
		return pathdom.Path{}, false
	}
	return pathdom.Path{
		Symbol:   key.Sym,
		Segments: resolver.KeySpace().Segments(key),
	}, true
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
