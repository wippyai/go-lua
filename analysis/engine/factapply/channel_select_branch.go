package factapply

import (
	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/channelselect"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/visibility"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	result, ok := resolvePathValueAt(reg, resolver, point, out, resultPath)
	if !ok {
		return out, false
	}
	caseType, ok := channelSelectResultCaseTypeFromValue(reg, result.value, selectFact.Select, selectFact.Index)
	if !ok {
		return out, false
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, caseType), caseType)
	out = invalidateChannelSelectResultDescendants(resolver, point, out, resultPath)
	return result.write(out, value), true
}

func channelSelectResultPathFromChannel(p pathdom.Path) (pathdom.Path, bool) {
	return channelselect.ResultPathFromChannelField(p)
}

func channelSelectReceiveFact(
	out state.State,
	resultKey pathdom.PathKey,
	caseKey pathdom.PathKey,
) (state.ChannelSelectFact, bool) {
	snapshot := out.ChannelSelectFactsSnapshot()
	if snapshot.Bottom {
		return state.ChannelSelectFact{}, false
	}
	for _, fact := range snapshot.Facts {
		if fact.Kind == state.ChannelSelectFactReceive && fact.Result == resultKey && fact.Case == caseKey {
			return fact, true
		}
	}
	return state.ChannelSelectFact{}, false
}

func channelSelectResultCaseTypeFromValue(
	reg *axis.Registry,
	value product.Value,
	selectID state.ChannelSelectID,
	index int,
) (typ.Type, bool) {
	resultType, ok := valueWitnessType(reg, value)
	if !ok {
		return nil, false
	}
	resultType = unwrap.Annotations(resultType)
	if union, ok := resultType.(*typ.Union); ok {
		for _, member := range union.Members {
			if channelSelectCaseTypeMatches(member, selectID, index) {
				return member, true
			}
		}
		return nil, false
	}
	if channelSelectCaseTypeMatches(resultType, selectID, index) {
		return resultType, true
	}
	return nil, false
}

func channelSelectCaseTypeMatches(caseType typ.Type, selectID state.ChannelSelectID, index int) bool {
	return channelselect.CaseTypeMatches(caseType, string(selectID), index)
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
