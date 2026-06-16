package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/channelselect"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func applyChannelSelectCaseEquality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (state.State, bool) {
	if updated, ok := applyChannelSelectCasePathEquality(reg, resolver, projectPath, point, out, leftPath, rightPath); ok {
		return updated, true
	}
	if updated, ok := applyChannelSelectCasePathEquality(reg, resolver, projectPath, point, out, rightPath, leftPath); ok {
		return updated, true
	}
	return out, false
}

func applyChannelSelectCasePathEquality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	resultChannelPath pathdom.Path,
	casePath pathdom.Path,
) (state.State, bool) {
	resultPath, ok := channelSelectResultPathFromChannel(resultChannelPath)
	if !ok || resolver == nil {
		return out, false
	}
	resultKey := resolver.KeyAt(point, resultPath)
	caseKey := resolver.KeyAt(point, casePath)
	if resultKey == "" || caseKey == "" {
		return out, false
	}
	selectFact, ok := channelSelectReceiveFact(out, resultKey, caseKey)
	if !ok {
		return out, false
	}
	result, ok := resolvePathValueAt(reg, resolver, point, out, resultPath, projectPath)
	if !ok {
		return out, false
	}
	resultType, ok := valueWitnessType(reg, result.value)
	var caseType typ.Type
	if ok {
		caseType, ok = channelselect.ResultCaseTypeFromValue(resultType, string(selectFact.Select), selectFact.Index)
		if !ok {
			if channelselect.ResultHasSelectID(resultType, string(selectFact.Select)) {
				return state.Domain(reg).Bottom(), true
			}
			return out, false
		}
	} else {
		caseType, ok = channelSelectPayloadCaseType(reg, selectFact)
		if !ok {
			return out, false
		}
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, caseType), caseType)
	out = invalidateChannelSelectResultDescendants(resolver, point, out, resultPath)
	return result.write(out, value), true
}

func channelSelectPayloadCaseType(reg *axis.Registry, fact channelselectfact.Fact) (typ.Type, bool) {
	payloadType, ok := channelSelectPayloadType(reg, fact)
	if !ok {
		return nil, false
	}
	return channelselect.ResultCaseType(string(fact.Select), fact.Index, payloadType), true
}

func channelSelectPayloadType(reg *axis.Registry, fact channelselectfact.Fact) (typ.Type, bool) {
	if !fact.HasPayload {
		return nil, false
	}
	return valueWitnessType(reg, fact.Payload)
}

func channelSelectRemainingTypeFromFacts(reg *axis.Registry, out state.State, selectID channelselectfact.ID, skipIndex int) (typ.Type, bool) {
	snapshot := out.ChannelSelectFactsSnapshot()
	if snapshot.Bottom {
		return nil, false
	}
	cases := make([]channelselect.ResultCase, 0)
	hasDefault := false
	for _, fact := range snapshot.Facts {
		if fact.Kind == channelselectfact.FactSelect && fact.Select == selectID && fact.HasDefault {
			hasDefault = true
			continue
		}
		if fact.Kind != channelselectfact.FactReceive || fact.Select != selectID || fact.Index == skipIndex {
			continue
		}
		payloadType, ok := channelSelectPayloadType(reg, fact)
		if !ok {
			continue
		}
		cases = append(cases, channelselect.ResultCase{
			Index:   fact.Index,
			Payload: payloadType,
		})
	}
	if len(cases) == 0 && !hasDefault {
		return typ.Never, true
	}
	return channelselect.ResultValueTypeWithDefault(string(selectID), cases, hasDefault)
}

func applyChannelSelectCaseInequality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (state.State, bool) {
	if updated, ok := applyChannelSelectCasePathInequality(reg, resolver, projectPath, point, out, leftPath, rightPath); ok {
		return updated, true
	}
	if updated, ok := applyChannelSelectCasePathInequality(reg, resolver, projectPath, point, out, rightPath, leftPath); ok {
		return updated, true
	}
	return out, false
}

func applyChannelSelectCasePathInequality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	resultChannelPath pathdom.Path,
	casePath pathdom.Path,
) (state.State, bool) {
	resultPath, ok := channelSelectResultPathFromChannel(resultChannelPath)
	if !ok || resolver == nil {
		return out, false
	}
	resultKey := resolver.KeyAt(point, resultPath)
	caseKey := resolver.KeyAt(point, casePath)
	if resultKey == "" || caseKey == "" {
		return out, false
	}
	selectFact, ok := channelSelectReceiveFact(out, resultKey, caseKey)
	if !ok {
		return out, false
	}
	result, ok := resolvePathValueAt(reg, resolver, point, out, resultPath, projectPath)
	if !ok {
		return out, false
	}
	resultType, ok := valueWitnessType(reg, result.value)
	var narrowed typ.Type
	if ok {
		narrowed, ok = channelselect.ResultWithoutCase(resultType, string(selectFact.Select), selectFact.Index)
		if !ok {
			return out, false
		}
	} else {
		narrowed, ok = channelSelectRemainingTypeFromFacts(reg, out, selectFact.Select, selectFact.Index)
		if !ok {
			return out, false
		}
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, narrowed), narrowed)
	out = invalidateChannelSelectResultDescendants(resolver, point, out, resultPath)
	return result.write(out, value), true
}

func channelSelectReceiveFact(
	out state.State,
	resultKey pathdom.PathKey,
	caseKey pathdom.PathKey,
) (channelselectfact.Fact, bool) {
	snapshot := out.ChannelSelectFactsSnapshot()
	if snapshot.Bottom {
		return channelselectfact.Fact{}, false
	}
	for _, fact := range snapshot.Facts {
		if fact.Kind == channelselectfact.FactReceive && fact.Result == resultKey && fact.Case == caseKey {
			return fact, true
		}
		if fact.Kind != channelselectfact.FactReceive || fact.Result == "" || fact.Case == "" {
			continue
		}
		rebasedResult, ok := rebasePathKeyToContext(fact.Result, resultKey)
		if !ok || rebasedResult != resultKey {
			continue
		}
		rebasedCase, ok := rebasePathKeyToContext(fact.Case, caseKey)
		if !ok || rebasedCase != caseKey {
			continue
		}
		return fact, true
	}
	return channelselectfact.Fact{}, false
}

func channelSelectResultPathFromChannel(p pathdom.Path) (pathdom.Path, bool) {
	seg, ok := p.LastSegment()
	if !ok || seg.Kind != segment.SegmentField || seg.Name != channelselect.ResultChannelField {
		return pathdom.Path{}, false
	}
	return p.Parent(), true
}

func rebasePathKeyToContext(pathKey pathdom.PathKey, contextKey pathdom.PathKey) (pathdom.PathKey, bool) {
	if pathKey == "" || contextKey == "" {
		return "", false
	}
	if pathKey == contextKey {
		return pathKey, true
	}
	fromSymbol, fromVersion, _, fromOK := pathaddr.ParseResolverPath(pathKey)
	toSymbol, toVersion, _, toOK := pathaddr.ParseResolverPath(contextKey)
	if !fromOK || !toOK || fromSymbol == 0 || toSymbol == 0 || fromSymbol != toSymbol {
		return "", false
	}
	fromRoot, ok := pathaddr.LocalKeyForVersion(fromSymbol, fromVersion, nil)
	if !ok {
		return "", false
	}
	toRoot, ok := pathaddr.LocalKeyForVersion(toSymbol, toVersion, nil)
	if !ok {
		return "", false
	}
	return pathaddr.RebasePathKey(pathKey, fromRoot.PathKey(), toRoot.PathKey())
}

func invalidateChannelSelectResultDescendants(
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	resultPath pathdom.Path,
) state.State {
	if len(resultPath.Segments) == 0 {
		return invalidateRootDescendantsAt(resolver, point, out, resultPath)
	}
	if invalidated, ok := invalidatePathDescendantsAt(out, resolver, point, resultPath); ok {
		return invalidated
	}
	return out
}
