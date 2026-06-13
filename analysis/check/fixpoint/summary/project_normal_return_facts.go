package summary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/state"
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
			admission, ok := projectDynamicIndexAdmission(stateFact.Admission)
			if !ok {
				continue
			}
			fact := DynamicIndexFact{
				Table:       table,
				Site:        string(stateKey.Site),
				KeyPresence: stateFact.KeyPresence,
				KeyValue:    stateFact.KeyValue,
				Value:       stateFact.Value,
				Admission:   admission,
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
			case BranchProofPathPresence:
				if stateProof.Presence.IsBottom() || stateProof.Presence.IsTop() {
					continue
				}
				proof.Presence = stateProof.Presence
			case BranchProofPathEqual, BranchProofPathNotEqual:
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
				Select: string(stateFact.Select),
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
			kind, ok := projectEffectDeltaKind(stateKey.Kind)
			if !ok {
				continue
			}
			change, ok := projectEffectDeltaChange(stateDelta.Change)
			if !ok {
				continue
			}
			delta := EffectDelta{
				Target: target,
				Site:   string(stateKey.Site),
				Kind:   kind,
				Before: stateDelta.Before,
				After:  stateDelta.After,
				Change: change,
			}
			if effectDeltaEqual(reg, delta, effectDeltaBottom(reg)) ||
				effectDeltaIsTop(reg, delta) {
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
	return fact.KeyPresence.IsTop() &&
		product.Equal(reg, fact.KeyValue, product.Top()) &&
		product.Equal(reg, fact.Value, product.Top()) &&
		fact.Admission == DynamicIndexAdmissionUnknown
}

func effectDeltaIsTop(reg *axis.Registry, delta EffectDelta) bool {
	return product.Equal(reg, delta.Before, product.Top()) &&
		product.Equal(reg, delta.After, product.Top()) &&
		delta.Change == EffectDeltaChangeUnknown
}

func projectDynamicIndexAdmission(admission state.DynamicIndexAdmission) (DynamicIndexAdmission, bool) {
	switch admission {
	case state.DynamicIndexAdmissionBottom:
		return DynamicIndexAdmissionBottom, true
	case state.DynamicIndexAdmissionAdmitted:
		return DynamicIndexAdmissionAdmitted, true
	case state.DynamicIndexAdmissionRejected:
		return DynamicIndexAdmissionRejected, true
	case state.DynamicIndexAdmissionUnknown:
		return DynamicIndexAdmissionUnknown, true
	default:
		return DynamicIndexAdmissionBottom, false
	}
}

func projectBranchProofKind(kind state.BranchProofKind) (BranchProofKind, bool) {
	switch kind {
	case state.BranchProofPathPresence:
		return BranchProofPathPresence, true
	case state.BranchProofPathEqual:
		return BranchProofPathEqual, true
	case state.BranchProofPathNotEqual:
		return BranchProofPathNotEqual, true
	default:
		return 0, false
	}
}

func projectChannelSelectKind(kind state.ChannelSelectFactKind) (ChannelSelectFactKind, bool) {
	switch kind {
	case state.ChannelSelectFactSelect:
		return ChannelSelectFactSelect, true
	case state.ChannelSelectFactReceive:
		return ChannelSelectFactReceive, true
	case state.ChannelSelectFactCase:
		return ChannelSelectFactCase, true
	default:
		return 0, false
	}
}

func projectEffectDeltaKind(kind state.EffectDeltaKind) (EffectDeltaKind, bool) {
	switch kind {
	case state.EffectDeltaMutation:
		return EffectDeltaMutation, true
	case state.EffectDeltaEscape:
		return EffectDeltaEscape, true
	case state.EffectDeltaCall:
		return EffectDeltaCall, true
	default:
		return 0, false
	}
}

func projectEffectDeltaChange(change state.EffectDeltaChange) (EffectDeltaChange, bool) {
	switch change {
	case state.EffectDeltaChangeBottom:
		return EffectDeltaChangeBottom, true
	case state.EffectDeltaChangeNone:
		return EffectDeltaChangeNone, true
	case state.EffectDeltaChangeChanged:
		return EffectDeltaChangeChanged, true
	case state.EffectDeltaChangeUnknown:
		return EffectDeltaChangeUnknown, true
	default:
		return EffectDeltaChangeBottom, false
	}
}
