package projectsummary

import (
	"github.com/wippyai/go-lua/analysis/domain/path"
	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/state/key"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/channelselectfact"
	effectdelta "github.com/wippyai/go-lua/analysis/engine/state/effectdelta"
	"github.com/wippyai/go-lua/analysis/engine/state/pathevidence"
)

func projectNormalReturnFacts(reg *axis.Registry, result ResultReader, exit state.State) callboundary.NormalReturnFacts {
	params := parameterValuePaths(result)
	if len(params) == 0 {
		return callboundary.NormalReturnFacts{}
	}
	projectPath := func(pathKey path.PathKey) (path.Path, bool) {
		return normalReturnFactPlaceholderPath(pathKey, params)
	}
	out := callboundary.NormalReturnFacts{}

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
			out.PathRefinements = append(out.PathRefinements, callboundary.PathValueFact{
				Path:  target,
				Value: portableBoundaryValue(reg, value),
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
			out.PathStaticMembers = append(out.PathStaticMembers, callboundary.PathStaticMemberFact{
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
			fact := callboundary.DynamicIndexFact{
				Table: table,
				Site:  stateKey.Site,
				Value: stateFact,
			}
			domain := dynamicindex.Domain(reg)
			if domain.Equal(fact.Value, dynamicindex.Bottom(reg)) || domain.Equal(fact.Value, dynamicindex.Top()) {
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
			proof := callboundary.BranchProof{
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
			fact := callboundary.ChannelSelectFact{
				Select:     channelselectfact.ID(stateFact.Select),
				Kind:       kind,
				Index:      stateFact.Index,
				HasDefault: stateFact.HasDefault,
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

	if snapshot := exit.FrozenTablesSnapshot(); !snapshot.Bottom && !snapshot.Top {
		frozenPaths := frozenTablePlaceholderPaths(reg, exit, params)
		for _, id := range snapshot.Tables {
			for _, target := range frozenPaths[id] {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
			}
		}
	}

	if snapshot := exit.EffectDeltasSnapshot(); !snapshot.Top {
		for stateKey, stateDelta := range snapshot.Deltas {
			target, ok := projectPath(stateKey.Target)
			if !ok {
				continue
			}
			if stateKey.Kind == effectdelta.Freeze && callboundary.IsFrozenTableEffectSite(stateKey.Site) {
				out.FrozenTables = append(out.FrozenTables, callboundary.FrozenTableFact{
					Target: target,
				})
				continue
			}
			if stateKey.Kind == effectdelta.Mutation && callboundary.IsPathInvalidationEffectSite(stateKey.Site) {
				out.PathInvalidations = append(out.PathInvalidations, callboundary.PathInvalidationFact{
					Path: target,
				})
				continue
			}
			if stateKey.Kind == effectdelta.Escape {
				if kind, recursive, ok := callboundary.EscapeEventFromEffectSite(stateKey.Site); ok {
					out.EscapeEvents = append(out.EscapeEvents, callboundary.EscapeEventFact{
						Target:    target,
						Kind:      kind,
						Recursive: recursive,
					})
					continue
				}
			}
			delta := callboundary.EffectDelta{
				Target: target,
				Site:   stateKey.Site,
				Kind:   stateKey.Kind,
				Value:  stateDelta,
			}
			domain := effectdelta.Domain(reg)
			if domain.Equal(delta.Value, domain.Bottom()) || domain.Equal(delta.Value, effectdelta.Top()) {
				continue
			}
			out.EffectDeltas = append(out.EffectDeltas, delta)
		}
	}

	if snapshot := exit.StoreRelationsSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for _, relation := range snapshot.Relations {
			source, ok := projectPath(relation.Source)
			if !ok {
				continue
			}
			into, ok := projectPath(relation.Into)
			if !ok {
				continue
			}
			out.StoreRelations = append(out.StoreRelations, callboundary.StoreRelationFact{
				Source: source,
				Into:   into,
			})
		}
	}

	return out
}

func frozenTablePlaceholderPaths(reg *axis.Registry, exit state.State, params []path.Path) map[identity.ID][]path.Path {
	out := make(map[identity.ID][]path.Path)
	var queue []frozenTablePathCandidate
	for i, param := range params {
		if param.Symbol == 0 {
			continue
		}
		value := exit.ReadValue(reg, key.SymbolValue(param.Symbol))
		id, ok := productIdentityID(reg, value)
		if !ok {
			continue
		}
		if addFrozenTablePlaceholderPath(out, id, path.NewPlaceholder(i)) {
			queue = append(queue, newFrozenTablePathCandidate(id, path.NewPlaceholder(i), nil))
		}
	}
	if snapshot := exit.PathRefinementsSnapshot(); !snapshot.Top {
		for pathKey, value := range snapshot.Refinements {
			id, ok := productIdentityID(reg, value)
			if !ok {
				continue
			}
			target, ok := normalReturnFactPlaceholderPath(pathKey, params)
			if !ok {
				continue
			}
			if addFrozenTablePlaceholderPath(out, id, target) {
				queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
			}
		}
	}
	if snapshot := exit.PathStaticMembersSnapshot(); !snapshot.Bottom && !snapshot.Top {
		for pathKey, value := range snapshot.Members {
			id, ok := productIdentityID(reg, value)
			if !ok {
				continue
			}
			target, ok := normalReturnFactPlaceholderPath(pathKey, params)
			if !ok {
				continue
			}
			if addFrozenTablePlaceholderPath(out, id, target) {
				queue = append(queue, newFrozenTablePathCandidate(id, target, nil))
			}
		}
	}
	heap := exit.HeapTableObjectsSnapshot()
	if heap.Top || len(heap.Objects) == 0 {
		return out
	}
	for len(queue) != 0 {
		candidate := queue[0]
		queue = queue[1:]
		object, ok := heap.Objects[candidate.id]
		if !ok {
			continue
		}
		for suffix, value := range object.StaticMembers() {
			childID, ok := productIdentityID(reg, value)
			if !ok || candidate.hasSeen(childID) {
				continue
			}
			segments, ok := segment.ParseFormattedSegments(string(suffix))
			if !ok {
				continue
			}
			childPath := appendPathSegments(candidate.path, segments)
			if addFrozenTablePlaceholderPath(out, childID, childPath) {
				queue = append(queue, newFrozenTablePathCandidate(childID, childPath, candidate.seen))
			}
		}
	}
	return out
}

type frozenTablePathCandidate struct {
	id   identity.ID
	path path.Path
	seen map[identity.ID]struct{}
}

func newFrozenTablePathCandidate(id identity.ID, target path.Path, seen map[identity.ID]struct{}) frozenTablePathCandidate {
	nextSeen := make(map[identity.ID]struct{}, len(seen)+1)
	for seenID := range seen {
		nextSeen[seenID] = struct{}{}
	}
	nextSeen[id] = struct{}{}
	return frozenTablePathCandidate{id: id, path: target, seen: nextSeen}
}

func (c frozenTablePathCandidate) hasSeen(id identity.ID) bool {
	_, ok := c.seen[id]
	return ok
}

func productIdentityID(reg *axis.Registry, value product.Value) (identity.ID, bool) {
	id, ok := product.Get(reg, value, identity.Key).ID()
	if !ok || id == (identity.ID{}) {
		return identity.ID{}, false
	}
	return id, true
}

func addFrozenTablePlaceholderPath(paths map[identity.ID][]path.Path, id identity.ID, target path.Path) bool {
	if id == (identity.ID{}) || target.IsEmpty() {
		return false
	}
	key := target.Key()
	for _, existing := range paths[id] {
		if existing.Key() == key {
			return false
		}
	}
	paths[id] = append(paths[id], target)
	return true
}

func appendPathSegments(base path.Path, segments []segment.Segment) path.Path {
	out := base
	for _, seg := range segments {
		out = out.Append(seg)
	}
	return out
}

func portableBoundaryValue(reg *axis.Registry, value product.Value) product.Value {
	return product.Set(reg, value, evidence.Key, evidence.Top())
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
