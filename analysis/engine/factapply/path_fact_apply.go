package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/heapidentity"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
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
	out = out.WritePathStaticMember(targetKey, value)
	if canonical, ok := pathaddr.FieldCanonicalPathKey(targetKey); ok {
		out = out.WritePathStaticMember(canonical, value)
	}
	return addPathEqualityProofFromSource(resolver, facts, ctx.Point, out, fact.TargetPath(), source)
}

func applyDynamicIndexWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	sources sourcevalue.SourceValues,
	read func(cfg.Point) state.State,
	in state.State,
	out state.State,
	fact factflow.DynamicIndexWrite,
) state.State {
	tableKey := factPathKeyAt(resolver, ctx.Point, fact.TablePath())
	if tableKey == "" {
		return out
	}
	key := dynamicindex.Key{
		Table: tableKey,
		Site:  dynamicindex.SiteForPoint(int(ctx.Point)),
	}
	value := dynamicIndexFact(ctx, sources, read, in, out, fact)
	out = out.WriteDynamicIndexFact(ctx.Registry, key, value)
	return writeHeapTableDynamicIndexFact(ctx, resolver, out, fact.TablePath(), key, value)
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
	out := dynamicindex.Fact{
		KeyPresence: presence.Bottom(),
		KeyValue:    product.Bottom(ctx.Registry),
		Value:       product.Bottom(ctx.Registry),
		Admission:   fact.Admission(),
	}
	readKey, readValue := dynamicIndexReadback(fact.ReadbackIntent())
	if readKey {
		keySource := fact.KeySource()
		if keyValue, ok := sources.ValueOfSource(ctx.Point, keySource, in, readWithSamePointCallSource(ctx.Point, keySource, read, current)); ok {
			out.KeyValue = keyValue
			out.KeyPresence = product.PresenceOf(keyValue)
		}
	}
	if readValue {
		source := fact.Source()
		if value, ok := sources.ValueOfSource(ctx.Point, source, in, readWithSamePointCallSource(ctx.Point, source, read, current)); ok {
			out.Value = value
		}
	}
	return out
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
	return out.AddBranchProof(stateProof)
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
	switch proof.Kind() {
	case factflow.BranchPathEvidencePresence:
		value, ok := proof.Presence()
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:     pathevidence.BranchProofPathPresence,
			Path:     pathKey,
			Presence: value,
		}, true
	case factflow.BranchPathEvidenceEqual:
		other, ok := branchPathEvidenceOtherPathKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathEqual,
			Path:  pathKey,
			Other: other,
		}, true
	case factflow.BranchPathEvidenceNotEqual:
		other, ok := branchPathEvidenceOtherPathKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofPathNotEqual,
			Path:  pathKey,
			Other: other,
		}, true
	case factflow.BranchPathEvidenceIndexInRange:
		other, ok := branchPathEvidenceOtherPathKeyAt(resolver, point, proof)
		if !ok {
			return pathevidence.BranchProof{}, false
		}
		return pathevidence.BranchProof{
			Kind:  pathevidence.BranchProofIndexInRange,
			Path:  pathKey,
			Other: other,
		}, true
	default:
		return pathevidence.BranchProof{}, false
	}
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
	if resolver == nil {
		return ""
	}
	return resolver.KeyAt(point, path)
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
	targetKey := factPathKeyAt(resolver, point, targetPath)
	sourceKey := factPathKeyAt(resolver, point, sourcePath)
	if targetKey == "" || sourceKey == "" || targetKey == sourceKey {
		return out
	}
	return out.AddBranchProof(pathevidence.BranchProof{
		Kind:  pathevidence.BranchProofPathEqual,
		Path:  targetKey,
		Other: sourceKey,
	})
}
