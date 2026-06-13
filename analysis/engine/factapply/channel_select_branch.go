package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func applyChannelSelectCaseEquality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	leftPath pathdom.Path,
	rightPath pathdom.Path,
) state.State {
	if updated, ok := applyChannelSelectCasePathEquality(reg, resolver, point, out, leftPath, rightPath); ok {
		return updated
	}
	if updated, ok := applyChannelSelectCasePathEquality(reg, resolver, point, out, rightPath, leftPath); ok {
		return updated
	}
	return out
}

func applyChannelSelectCasePathEquality(
	reg *axis.Registry,
	resolver *visibility.Resolver,
	point cfg.Point,
	out state.State,
	resultChannelPath pathdom.Path,
	casePath pathdom.Path,
) (state.State, bool) {
	resultPath, ok := channelselect.ResultPathFromChannel(resultChannelPath)
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
	result, ok := resolvePathValueAt(reg, resolver, point, out, resultPath)
	if !ok {
		return out, false
	}
	resultType, ok := valueWitnessType(reg, result.value)
	if !ok {
		return out, false
	}
	caseType, ok := channelselect.ResultCaseTypeFromValue(resultType, string(selectFact.Select), selectFact.Index)
	if !ok {
		return out, false
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, caseType), caseType)
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
	}
	return channelselectfact.Fact{}, false
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
