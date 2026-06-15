package factapply

import (
	"strconv"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	sourcevalue "github.com/wippyai/go-lua/analysis/engine/sourcevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
	"github.com/wippyai/go-lua/analysis/engine/transfer"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyPathStaticMemberWrite(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
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
	return out.WritePathStaticMember(targetKey, value)
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
	return out.WriteDynamicIndexFact(ctx.Registry, dynamicindex.Key{
		Table: tableKey,
		Site:  dynamicIndexSite(ctx.Point),
	}, dynamicIndexFact(ctx, sources, read, in, out, fact))
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

func dynamicIndexSite(point cfg.Point) dynamicindex.Site {
	return dynamicindex.Site("factflow.dynamic_index_write@" + strconv.Itoa(int(point)))
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
	out state.State,
	proof factflow.BranchPathEvidence,
) state.State {
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

func applyChannelSelect(
	ctx transfer.NodeContext,
	resolver *visibility.Resolver,
	out state.State,
	event factflow.ChannelSelect,
) state.State {
	fact, ok := channelSelectFactAt(resolver, ctx.Point, event)
	if !ok {
		return out
	}
	return out.AddChannelSelectFact(fact)
}

func channelSelectFactAt(
	resolver *visibility.Resolver,
	point cfg.Point,
	event factflow.ChannelSelect,
) (channelselectfact.Fact, bool) {
	kind, ok := channelSelectKind(event.Kind())
	if !ok {
		return channelselectfact.Fact{}, false
	}
	fact := channelselectfact.Fact{
		Select: channelselectfact.ID(event.SelectID()),
		Kind:   kind,
		Index:  event.Index(),
	}
	if resultPath, ok := event.ResultPath(); ok {
		fact.Result = factPathKeyAt(resolver, point, resultPath)
		if fact.Result == "" {
			return channelselectfact.Fact{}, false
		}
	}
	if casePath, ok := event.CasePath(); ok {
		fact.Case = factPathKeyAt(resolver, point, casePath)
		if fact.Case == "" {
			return channelselectfact.Fact{}, false
		}
	}
	if payload, ok := event.PayloadValue(); ok {
		fact.Payload = payload
		fact.HasPayload = true
	}
	return fact, true
}

func channelSelectKind(kind factflow.ChannelSelectKind) (channelselectfact.Kind, bool) {
	switch kind {
	case factflow.ChannelSelectSelect:
		return channelselectfact.FactSelect, true
	case factflow.ChannelSelectReceive:
		return channelselectfact.FactReceive, true
	case factflow.ChannelSelectCase:
		return channelselectfact.FactCase, true
	default:
		return 0, false
	}
}

func factPathKeyAt(resolver *visibility.Resolver, point cfg.Point, path pathdom.Path) pathdom.PathKey {
	if resolver == nil {
		return ""
	}
	return resolver.KeyAt(point, path)
}
