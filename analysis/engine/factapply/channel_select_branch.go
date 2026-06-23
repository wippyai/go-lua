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
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func applyChannelSelectCaseEquality(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (state.State, bool) {
	return applyChannelSelectCaseSymmetric(out, leftPath, rightPath, func(l, r pathdom.Path) (state.State, bool) {
		return applyChannelSelectCasePathEquality(typeValues, reg, resolver, projectPath, point, out, l, r)
	})
}

// applyChannelSelectCaseSymmetric applies apply to the path pair in both orders,
// returning the first successful update or (out, false) when neither applies.
func applyChannelSelectCaseSymmetric(
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
	apply func(leftPath, rightPath pathdom.Path) (state.State, bool),
) (state.State, bool) {
	if updated, ok := apply(leftPath, rightPath); ok {
		return updated, true
	}
	if updated, ok := apply(rightPath, leftPath); ok {
		return updated, true
	}
	return out, false
}

func applyChannelSelectCasePathEquality(
	typeValues *typevalue.Cache,
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
	resultKey, resultKeyOK := resolver.StateKeyAt(point, resultPath)
	caseKey, caseKeyOK := resolver.StateKeyAt(point, casePath)
	if !resultKeyOK || !caseKeyOK {
		return out, false
	}
	selectFacts := channelSelectReceiveFacts(out, resultKey, caseKey)
	if len(selectFacts) == 0 {
		return out, false
	}
	result, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, resultPath, projectPath)
	if !ok {
		return out, false
	}
	resultType, ok := valueWitnessType(reg, result.value)
	var caseTypes []typ.Type
	if ok {
		missingKnownSelect := false
		for _, selectFact := range selectFacts {
			caseType, ok := channelselect.ResultCaseTypeFromValue(resultType, string(selectFact.Select), selectFact.Index)
			if ok {
				if payloadType, payloadOK := channelSelectExactPayloadType(reg, selectFact); payloadOK {
					caseType = channelselect.ResultCaseType(string(selectFact.Select), selectFact.Index, payloadType)
				}
				caseTypes = append(caseTypes, caseType)
				continue
			}
			if channelselect.ResultHasSelectID(resultType, string(selectFact.Select)) {
				missingKnownSelect = true
			}
		}
		if len(caseTypes) == 0 {
			if missingKnownSelect {
				return state.Domain(reg).Bottom(), true
			}
			return out, false
		}
	} else {
		for _, selectFact := range selectFacts {
			caseTypes = append(caseTypes, channelSelectPayloadCaseType(reg, selectFact))
		}
		if len(caseTypes) == 0 {
			return out, false
		}
	}
	value := typeValues.FromTypeWithWitness(reg, typeexpr.Union(caseTypes...))
	out = invalidateChannelSelectResultDescendants(resolver, point, out, resultPath)
	return result.write(out, value), true
}

func channelSelectPayloadCaseType(reg *axis.Registry, fact channelselectfact.Fact) typ.Type {
	return channelselect.ResultCaseType(string(fact.Select), fact.Index, channelSelectPayloadType(reg, fact))
}

func channelSelectPayloadType(reg *axis.Registry, fact channelselectfact.Fact) typ.Type {
	if payloadType, ok := channelSelectExactPayloadType(reg, fact); ok {
		return payloadType
	}
	return typ.Unknown
}

func channelSelectExactPayloadType(reg *axis.Registry, fact channelselectfact.Fact) (typ.Type, bool) {
	if !fact.HasPayload {
		return nil, false
	}
	return valueWitnessType(reg, fact.Payload)
}

func channelSelectRemainingTypeFromFacts(reg *axis.Registry, out state.State, selectID channelselectfact.ID, skipIndexes map[int]bool) (typ.Type, bool) {
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
		if fact.Kind != channelselectfact.FactReceive || fact.Select != selectID || skipIndexes[fact.Index] {
			continue
		}
		cases = append(cases, channelselect.ResultCase{
			Index:   fact.Index,
			Payload: channelSelectPayloadType(reg, fact),
		})
	}
	if len(cases) == 0 && !hasDefault {
		return typ.Never, true
	}
	return channelselect.ResultValueTypeWithDefault(string(selectID), cases, hasDefault)
}

func applyChannelSelectCaseInequality(
	typeValues *typevalue.Cache,
	reg *axis.Registry,
	resolver *visibility.Resolver,
	projectPath PathTypeProjector,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) (state.State, bool) {
	return applyChannelSelectCaseSymmetric(out, leftPath, rightPath, func(l, r pathdom.Path) (state.State, bool) {
		return applyChannelSelectCasePathInequality(typeValues, reg, resolver, projectPath, point, out, l, r)
	})
}

func applyChannelSelectCasePathInequality(
	typeValues *typevalue.Cache,
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
	resultKey, resultKeyOK := resolver.StateKeyAt(point, resultPath)
	caseKey, caseKeyOK := resolver.StateKeyAt(point, casePath)
	if !resultKeyOK || !caseKeyOK {
		return out, false
	}
	selectFacts := channelSelectReceiveFacts(out, resultKey, caseKey)
	if len(selectFacts) == 0 {
		return out, false
	}
	result, ok := resolvePathValueAtCached(typeValues, reg, resolver, point, out, resultPath, projectPath)
	if !ok {
		return out, false
	}
	resultType, ok := valueWitnessType(reg, result.value)
	var narrowed typ.Type
	if ok {
		narrowed = resultType
		removed := false
		for _, selectFact := range selectFacts {
			next, ok := channelselect.ResultWithoutCase(narrowed, string(selectFact.Select), selectFact.Index)
			if !ok {
				continue
			}
			narrowed = next
			removed = true
		}
		if !removed {
			return out, false
		}
	} else {
		narrowed, ok = channelSelectRemainingTypeFromFacts(reg, out, selectFacts[0].Select, channelSelectFactIndexes(selectFacts))
		if !ok {
			return out, false
		}
	}
	value := typeValues.FromTypeWithWitness(reg, narrowed)
	out = invalidateChannelSelectResultDescendants(resolver, point, out, resultPath)
	return result.write(out, value), true
}

func channelSelectReceiveFacts(
	out state.State,
	resultKey pathaddr.StateKey,
	caseKey pathaddr.StateKey,
) []channelselectfact.Fact {
	snapshot := out.ChannelSelectFactsSnapshot()
	if snapshot.Bottom {
		return nil
	}
	var outFacts []channelselectfact.Fact
	for _, fact := range snapshot.Facts {
		if fact.Kind == channelselectfact.FactReceive && fact.Result == resultKey && fact.Case == caseKey {
			outFacts = append(outFacts, fact)
			continue
		}
		if fact.Kind != channelselectfact.FactReceive || fact.Result == "" || fact.Case == "" {
			continue
		}
		rebasedResult, ok := pathaddr.RebaseLocalPathKeyToContext(fact.Result.PathKey(), resultKey.PathKey())
		if !ok || rebasedResult != resultKey.PathKey() {
			continue
		}
		rebasedCase, ok := pathaddr.RebaseLocalPathKeyToContext(fact.Case.PathKey(), caseKey.PathKey())
		if !ok || rebasedCase != caseKey.PathKey() {
			continue
		}
		outFacts = append(outFacts, fact)
	}
	return outFacts
}

func channelSelectFactIndexes(facts []channelselectfact.Fact) map[int]bool {
	indexes := make(map[int]bool, len(facts))
	for _, fact := range facts {
		indexes[fact.Index] = true
	}
	return indexes
}

func channelSelectResultPathFromChannel(p pathdom.Path) (pathdom.Path, bool) {
	seg, ok := p.LastSegment()
	if !ok || seg.Kind != segment.SegmentField || seg.Name != channelselect.ResultChannelField {
		return pathdom.Path{}, false
	}
	return p.Parent(), true
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
