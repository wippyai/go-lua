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
	domain := state.RegisteredProductDomain(ctx.Registry)
	for parent := targetPath.Parent(); !parent.IsEmpty(); parent = parent.Parent() {
		if next, _, err := applyValueRefinementFactorState(
			domain, nil, resolver, nil, ctx.Point, out, parent, factflow.NewValueConstraint(present), false,
		); err == nil {
			out = next
		}
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
	switch proof.Kind() {
	case factflow.BranchPathEvidenceEqual:
		if other, ok := proof.OtherPathRef(); ok {
			if selected, applied := applyChannelSelectCaseEquality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.PathRef(), other); applied {
				out = selected
			} else {
				if next, _, err := applyPathEqualityFactorState(
					state.RegisteredProductDomain(ctx.Registry), typeValues, resolver, ctx.Edge.From, out, proof.PathRef(), other,
				); err == nil {
					out = next
				}
			}
			if stateIsBottom(ctx.Registry, out) {
				return out
			}
		}
	case factflow.BranchPathEvidenceNotEqual:
		if other, ok := proof.OtherPathRef(); ok {
			if selected, applied := applyChannelSelectCaseInequality(typeValues, ctx.Registry, resolver, projectPath, ctx.Edge.From, out, proof.PathRef(), other); applied {
				out = selected
				if stateIsBottom(ctx.Registry, out) {
					return out
				}
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

func closeBranchProofsAcrossEquality(ks *keyspace.KeySpace, out state.State, aKey, bKey keyspace.Key) state.State {
	if ks == nil || aKey == bKey {
		return out
	}
	proofs := out.BranchProofsSnapshot(ks).Proofs
	synthetic := pathevidence.BranchProof{Kind: pathevidence.BranchProofPathEqual, Path: aKey, Other: bKey}
	proofs = append(proofs, synthetic)
	closed := pathevidence.CloseBranchProofsAcrossKnownEqualities(ks, proofs)
	additions := closed[:0]
	for _, proof := range closed {
		if proof != synthetic {
			additions = append(additions, proof)
		}
	}
	return out.AddBranchProofs(additions)
}

func closeBranchProofsAcrossKnownEqualities(ks *keyspace.KeySpace, out state.State) state.State {
	if ks == nil {
		return out
	}
	proofs := out.BranchProofsSnapshot(ks).Proofs
	return out.AddBranchProofs(pathevidence.CloseBranchProofsAcrossKnownEqualities(ks, proofs))
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
	if sym, segments, ok := pathaddr.ParseSymbolPathKey(source.PathKey); ok {
		return pathdom.Path{
			Symbol:   sym,
			Segments: segments,
		}, true
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
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	targetPath pathdom.Path,
	sourcePath pathdom.Path,
) state.State {
	targetKey, targetOK := visibility.AddressAt(resolver, point, targetPath).VisibleKeyspaceKey()
	sourceKey, sourceOK := visibility.AddressAt(resolver, point, sourcePath).VisibleKeyspaceKey()
	if !targetOK || !sourceOK || targetKey == sourceKey {
		return out
	}
	proof := pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  targetKey,
		Other: sourceKey,
	}
	domain := state.RegisteredProductDomain(reg)
	written, err := domain.ApplyPathEqualityProof(resolver.KeySpace(), proof, out)
	if err != nil {
		return out
	}
	return written
}
