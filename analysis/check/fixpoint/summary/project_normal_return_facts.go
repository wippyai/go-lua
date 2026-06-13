package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	"github.com/wippyai/go-lua/analysis/engine/state/dynamicindex"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func projectNormalReturnFacts(reg *axis.Registry, result ResultReader, exit state.State) NormalReturnFacts {
	params := normalReturnParamPaths(result)
	if len(params) == 0 {
		return NormalReturnFacts{}
	}
	projectPath := func(pathKey path.PathKey) (path.Path, bool) {
		return normalReturnFactPlaceholderPath(pathKey, params)
	}
	out := NormalReturnFacts{}

	if snapshot := exit.PathRefinementsSnapshot(); !snapshot.Top {
		bottom := product.Bottom(reg)
		top := product.Top()
		for pathKey, value := range snapshot.Refinements {
			if product.Equal(reg, value, bottom) || product.Equal(reg, value, top) {
				continue
			}
			target, ok := projectPath(pathKey)
			if !ok {
				continue
			}
			out.PathRefinements = append(out.PathRefinements, PathValueFact{
				Path:  target,
				Value: value,
			})
		}
	}

	if snapshot := exit.PathStaticMembersSnapshot(); !snapshot.Bottom && !snapshot.Top {
		bottom := product.Bottom(reg)
		for pathKey, value := range snapshot.Members {
			if product.Equal(reg, value, bottom) {
				continue
			}
			target, ok := projectPath(pathKey)
			if !ok {
				continue
			}
			out.PathStaticMembers = append(out.PathStaticMembers, PathStaticMemberFact{
				Path:  target,
				Value: value,
			})
		}
	}

	if snapshot := exit.DynamicIndexFactsSnapshot(); !snapshot.Top {
		for stateKey, stateFact := range snapshot.Facts {
			table, ok := projectPath(stateKey.Table)
			if !ok {
				continue
			}
			fact := DynamicIndexFact{
				Table: table,
				Site:  stateKey.Site,
				Value: stateFact,
			}
			if dynamicIndexFactEqual(reg, fact, dynamicIndexFactBottom(reg)) ||
				dynamicIndexFactIsTop(reg, fact) {
				continue
			}
			out.DynamicIndexFacts = append(out.DynamicIndexFacts, fact)
		}
	}

	if snapshot := exit.BranchProofsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, stateProof := range snapshot.Proofs {
			target, ok := projectPath(stateProof.Path)
			if !ok {
				continue
			}
			kind, ok := projectBranchProofKind(stateProof.Kind)
			if !ok {
				continue
			}
			proof := BranchProof{
				Kind: kind,
				Path: target,
			}
			switch kind {
			case pathevidence.BranchProofPathPresence:
				if stateProof.Presence.IsBottom() || stateProof.Presence.IsTop() {
					continue
				}
				proof.Presence = stateProof.Presence
			case pathevidence.BranchProofPathEqual, pathevidence.BranchProofPathNotEqual:
				other, ok := projectPath(stateProof.Other)
				if !ok {
					continue
				}
				proof.Other = other
			default:
				continue
			}
			out.BranchProofs = append(out.BranchProofs, proof)
		}
	}

	if snapshot := exit.ChannelSelectFactsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, stateFact := range snapshot.Facts {
			kind, ok := projectChannelSelectKind(stateFact.Kind)
			if !ok {
				continue
			}
			fact := ChannelSelectFact{
				Select: channelselectfact.ID(stateFact.Select),
				Kind:   kind,
				Index:  stateFact.Index,
			}
			if stateFact.Result != "" {
				resultPath, ok := projectPath(stateFact.Result)
				if !ok {
					continue
				}
				fact.Result = resultPath
			}
			if stateFact.Case != "" {
				casePath, ok := projectPath(stateFact.Case)
				if !ok {
					continue
				}
				fact.Case = casePath
			}
			out.ChannelSelects = append(out.ChannelSelects, fact)
		}
	}

	if snapshot := exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey, stateDelta := range snapshot.Deltas {
			target, ok := projectPath(stateKey.Target)
			if !ok {
				continue
			}
			delta := EffectDelta{
				Target: target,
				Site:   stateKey.Site,
				Kind:   stateKey.Kind,
				Value:  stateDelta,
			}
			if effectDeltaEqual(reg, delta, EffectDelta{Value: effectdelta.Bottom(reg)}) ||
				effectDeltaIsTop(delta) {
				continue
			}
			out.EffectDeltas = append(out.EffectDeltas, delta)
		}
	}

	return out
}

func normalReturnFactPlaceholderPath(pathKey path.PathKey, params []path.Path) (path.Path, bool) {
	if pathKey == "" || len(params) == 0 {
		return path.Path{}, false
	}
	if index, suffix, ok := normalReturnPlaceholderPathKey(pathKey); ok {
		if index >= len(params) || params[index].IsEmpty() {
			return path.Path{}, false
		}
		return normalReturnFactPlaceholderPathWithSuffix(index, suffix)
	}
	sym, version, suffix, ok := pathaddr.ParseResolverPath(pathKey)
	if !ok || version <= 0 {
		return path.Path{}, false
	}
	for i, param := range params {
		if param.Symbol == 0 || param.Symbol != sym {
			continue
		}
		return normalReturnFactPlaceholderPathWithSuffix(i, suffix)
	}
	return path.Path{}, false
}

func normalReturnPlaceholderPathKey(pathKey path.PathKey) (index int, suffix string, ok bool) {
	s := string(pathKey)
	if len(s) < 2 || s[0] != '$' {
		return 0, "", false
	}
	end := 1
	for end < len(s) && s[end] >= '0' && s[end] <= '9' {
		end++
	}
	if end == 1 {
		return 0, "", false
	}
	root := s[:end]
	index = path.PlaceholderIndexFromString(root)
	if index < 0 || path.NewPlaceholder(index).Root != root {
		return 0, "", false
	}
	return index, s[end:], true
}

func normalReturnFactPlaceholderPathWithSuffix(index int, suffix string) (path.Path, bool) {
	segments, ok := segment.ParseFormattedSegments(suffix)
	if !ok {
		return path.Path{}, false
	}
	out := path.NewPlaceholder(index)
	for _, seg := range segments {
		out = out.Append(seg)
	}
	return out, true
}

func dynamicIndexFactIsTop(reg *axis.Registry, fact DynamicIndexFact) bool {
	return dynamicindex.Domain(reg).Equal(fact.Value, dynamicindex.Top())
}

func effectDeltaIsTop(delta EffectDelta) bool {
	return delta.Value == effectdelta.Top()
}

func projectBranchProofKind(kind pathevidence.BranchProofKind) (pathevidence.BranchProofKind, bool) {
	switch kind {
	case pathevidence.BranchProofPathPresence:
		return pathevidence.BranchProofPathPresence, true
	case pathevidence.BranchProofPathEqual:
		return pathevidence.BranchProofPathEqual, true
	case pathevidence.BranchProofPathNotEqual:
		return pathevidence.BranchProofPathNotEqual, true
	default:
		return 0, false
	}
}

func projectChannelSelectKind(kind channelselectfact.Kind) (channelselectfact.Kind, bool) {
	switch kind {
	case channelselectfact.FactSelect:
		return channelselectfact.FactSelect, true
	case channelselectfact.FactReceive:
		return channelselectfact.FactReceive, true
	case channelselectfact.FactCase:
		return channelselectfact.FactCase, true
	default:
		return 0, false
	}
}
