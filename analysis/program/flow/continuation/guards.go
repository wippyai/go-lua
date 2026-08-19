package continuation

import (
	"errors"

	"github.com/wippyai/go-lua/analysis/program/flow/candidates"
	"github.com/wippyai/go-lua/analysis/program/flow/causal"
	"github.com/wippyai/go-lua/analysis/program/flow/executable"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

const noGuardRank = ^uint32(0)

type guardSeal struct {
	roots    [keyspace.FamilyCount][]uint32
	families [keyspace.FamilyCount]bool
	nodes    []guardNode
	counts   [keyspace.FamilyCount]uint32
	base     [keyspace.FamilyCount]uint32
	terms    uint32
	exec     *executable.Result
	cand     *candidates.Result
	causal   *causal.Result
	source   source.View
}

type guardRoute struct {
	to        uint32
	decision  uint32
	successor causal.Successor
}

type guardRange struct{ start, past uint32 }

func newGuardSeal(input inputProof) (*guardSeal, error) {
	seal := &guardSeal{counts: input.counts, exec: input.exec, cand: input.cand, causal: input.causal, source: input.source}
	for _, family := range continuationSubjects {
		if input.counts[family] == ^uint32(0) || uint64(input.counts[family])+1 > uint64(^uint(0)>>1) {
			return nil, errors.New("program/flow/continuation: Guard subject denominator is too large")
		}
		seal.families[family] = true
		seal.roots[family] = make([]uint32, input.counts[family]+1)
		for ordinal := range seal.roots[family] {
			seal.roots[family][ordinal] = absentRoot
		}
	}
	if err := seal.layout(); err != nil {
		return nil, err
	}
	return seal, nil
}

func (seal *guardSeal) layout() error {
	if seal == nil || seal.causal == nil {
		return errors.New("program/flow/continuation: Guard owner is unavailable")
	}
	var total uint64
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if total > uint64(^uint32(0)) {
			return errors.New("program/flow/continuation: causal vertex space is too large")
		}
		seal.base[family] = uint32(total)
		total += uint64(seal.counts[family])
	}
	if total == 0 || total > uint64(^uint32(0)) || total > uint64(^uint(0)>>1) {
		return errors.New("program/flow/continuation: invalid causal vertex denominator")
	}
	seal.terms = uint32(total)
	return seal.solve()
}

// solve distributes the Guard equation one decision at a time.  For a fixed
// d, Decision edges seed d, and d follows each local edge except an exact Mu
// reset of d; a same-d Decision reintroduces it.  This is algebraically equal
// to (incoming \ reset) union Decision, while storing no V-by-D plane.
func (seal *guardSeal) solve() error {
	vertices := make([]keyspace.Term, seal.terms)
	subject := make([]bool, seal.terms)
	// Causal's guarded Body route is the lexical entry witness for every
	// candidate subject fronted by that Body. Keep this reverse frontier only
	// during solving so a decision entering a child Body reaches its subjects
	// without retaining another graph projection in Result.
	bodySubjects := make([][]uint32, seal.counts[keyspace.FamilyBody]+1)
	for index := uint32(0); index < seal.terms; index++ {
		term, ok := seal.termAt(index)
		if !ok {
			return errors.New("program/flow/continuation: causal vertex is unavailable")
		}
		vertices[index] = term
		subject[index] = subjectFrom(seal.exec, seal.cand, term)
		if !subject[index] {
			continue
		}
		body, _, frontierOK := seal.source.Index().Frontier(term)
		if !frontierOK || !keyspace.ValidTerm(body, keyspace.FamilyBody, int(seal.counts[keyspace.FamilyBody])) {
			return errors.New("program/flow/continuation: Guard subject lacks Source Frontier")
		}
		bodySubjects[keyspace.TermOrdinal(body)] = append(bodySubjects[keyspace.TermOrdinal(body)], index)
	}
	successors := seal.causal.Successors()
	// endpoint is construction scratch for the additional query denominator.
	// The published owner keeps only endpoint -> existing prefix-root entries;
	// it never retains this mark plane, the route table, or a V-by-D matrix.
	endpoint := make([]bool, seal.terms)
	ranges := make([]guardRange, seal.terms)
	routes := make([]guardRoute, 0, seal.terms)
	decisionCount := uint64(seal.counts[keyspace.FamilySelect]) + uint64(seal.counts[keyspace.FamilyBranch]) + uint64(seal.counts[keyspace.FamilyLoop])
	if decisionCount > uint64(^uint32(0)) || decisionCount > uint64(^uint(0)>>1) {
		return errors.New("program/flow/continuation: Guard decision denominator is too large")
	}
	decisionCount32 := uint32(decisionCount)
	decisionCountInt := int(decisionCount)
	decisionTerms := make([]keyspace.Term, 0, decisionCountInt)
	maximumDecisionOrdinal := maxUint32(seal.counts[keyspace.FamilySelect], maxUint32(seal.counts[keyspace.FamilyBranch], seal.counts[keyspace.FamilyLoop]))
	for ordinal := uint32(1); ordinal <= maximumDecisionOrdinal; ordinal++ {
		for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
			if ordinal <= seal.counts[family] {
				decisionTerms = append(decisionTerms, keyspace.MakeTerm(family, ordinal))
			}
		}
	}
	if uint64(len(decisionTerms)) != decisionCount {
		return errors.New("program/flow/continuation: Guard decision order is malformed")
	}
	seedHeads := make([]uint32, decisionCountInt)
	seedNext := make([]uint32, 0, seal.terms)
	for fromIndex, from := range vertices {
		if uint64(len(routes)) > uint64(^uint32(0)) || uint64(len(routes)) > uint64(^uint(0)>>1) {
			return errors.New("program/flow/continuation: Guard route denominator is too large")
		}
		startLength := uint64(len(routes))
		if startLength > uint64(^uint32(0)) || startLength > uint64(^uint(0)>>1) {
			return errors.New("program/flow/continuation: Guard route denominator is too large")
		}
		start := uint32(startLength)
		for offset := 0; offset < successors.Count(from); offset++ {
			successor, ok := successors.At(from, offset)
			if !ok {
				return errors.New("program/flow/continuation: causal Successor is unavailable")
			}
			to, ok := seal.termIndex(successor.To)
			if !ok {
				return errors.New("program/flow/continuation: causal Successor leaves Source denominator")
			}
			if err := seal.validateTransfer(successor); err != nil {
				return err
			}
			// Both ends of every retained semantic route are admitted. This is
			// derived from the existing Causal.Successors stream; no second
			// endpoint graph is retained by Continuation.
			endpoint[fromIndex] = true
			endpoint[to] = true
			route := guardRoute{to: to, decision: noGuardRank, successor: successor}
			if successor.Decision != 0 {
				rank, valid := guardRank(successor.Decision, seal.counts)
				if !valid {
					return errors.New("program/flow/continuation: causal Decision is malformed")
				}
				route.decision = rank
			}
			if uint64(len(routes)) >= uint64(^uint32(0)) || uint64(len(routes)) >= uint64(^uint(0)>>1) {
				return errors.New("program/flow/continuation: Guard route denominator is too large")
			}
			routeIndex := uint32(len(routes))
			routes = append(routes, route)
			seedNext = append(seedNext, 0)
			if route.decision != noGuardRank {
				seedNext[routeIndex] = seedHeads[route.decision]
				seedHeads[route.decision] = routeIndex + 1
			}
		}
		pastLength := uint64(len(routes))
		if pastLength > uint64(^uint32(0)) || pastLength > uint64(^uint(0)>>1) {
			return errors.New("program/flow/continuation: Guard route denominator is too large")
		}
		ranges[fromIndex] = guardRange{start: start, past: uint32(pastLength)}
	}
	// Admit every exact Causal Site, including a body-terminal Outcome that
	// has no Successor route. The site projection is the upstream denominator;
	// Continuation retains only its own root projection. Route-reachable sites
	// already accumulated propagated guards above, while a terminal-only site
	// keeps subjectRoots at zero, the valid present-empty Guard scope.
	for siteIndex := 0; siteIndex < seal.causal.SiteCount(); siteIndex++ {
		site, ok := seal.causal.SiteAt(siteIndex)
		if !ok {
			return errors.New("program/flow/continuation: causal Site is unavailable")
		}
		term, ok := site.Term()
		if !ok {
			return errors.New("program/flow/continuation: causal Site term is unavailable")
		}
		vertex, ok := seal.termIndex(term)
		if !ok {
			return errors.New("program/flow/continuation: causal Site leaves Source denominator")
		}
		endpoint[vertex] = true
	}

	subjectRoots := make([]uint32, seal.terms)
	builder := newGuardChainBuilder()
	extendSubject := func(vertex uint32, term keyspace.Term) error {
		if !subject[vertex] && !endpoint[vertex] {
			return nil
		}
		parent := subjectRoots[vertex]
		root, ok := builder.append(parent, term)
		if !ok {
			return errors.New("program/flow/continuation: Guard prefix is malformed or too large")
		}
		subjectRoots[vertex] = root
		return nil
	}

	marks := make([]uint32, seal.terms)
	queue := make([]uint32, 0, seal.terms)
	generation := uint32(0)
	for rank := uint32(0); rank < decisionCount32; rank++ {
		generation++
		if generation == 0 {
			for index := range marks {
				marks[index] = 0
			}
			generation = 1
		}
		builder.beginRank()
		term := decisionTerms[rank]
		queue = queue[:0]
		for entry := seedHeads[rank]; entry != 0; entry = seedNext[entry-1] {
			route := routes[entry-1]
			if marks[route.to] == generation {
				continue
			}
			marks[route.to] = generation
			queue = append(queue, route.to)
		}
		for cursor := 0; cursor < len(queue); cursor++ {
			from := queue[cursor]
			if err := extendSubject(from, term); err != nil {
				return err
			}
			if keyspace.TermFamily(vertices[from]) == keyspace.FamilyBody {
				bodyOrdinal := keyspace.TermOrdinal(vertices[from])
				if bodyOrdinal != 0 && uint64(bodyOrdinal) < uint64(len(bodySubjects)) {
					for _, subjectIndex := range bodySubjects[bodyOrdinal] {
						if marks[subjectIndex] == generation {
							continue
						}
						marks[subjectIndex] = generation
						queue = append(queue, subjectIndex)
					}
				}
			}
			for routeIndex := ranges[from].start; routeIndex < ranges[from].past; routeIndex++ {
				route := routes[routeIndex]
				if !guardRouteAdmits(route, rank, term) {
					continue
				}
				if marks[route.to] == generation {
					continue
				}
				marks[route.to] = generation
				queue = append(queue, route.to)
			}
		}
	}

	// Allocate only family planes that actually contain a subject or route
	// endpoint. Subject families were allocated up front; endpoint-only
	// families are added here after the existing successor stream has been
	// validated.
	for index, term := range vertices {
		if !subject[index] && !endpoint[index] {
			continue
		}
		family := keyspace.TermFamily(term)
		if seal.families[family] {
			continue
		}
		seal.families[family] = true
		if seal.counts[family] == ^uint32(0) || uint64(seal.counts[family])+1 > uint64(^uint(0)>>1) {
			return errors.New("program/flow/continuation: Guard endpoint denominator is too large")
		}
		seal.roots[family] = make([]uint32, seal.counts[family]+1)
		for ordinal := range seal.roots[family] {
			seal.roots[family][ordinal] = absentRoot
		}
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		if !seal.families[family] {
			continue
		}
		for ordinal := uint32(1); ordinal <= seal.counts[family]; ordinal++ {
			term := keyspace.MakeTerm(family, ordinal)
			index, ok := seal.termIndex(term)
			if !ok {
				return errors.New("program/flow/continuation: subject index is unavailable")
			}
			if !subject[index] && !endpoint[index] {
				continue
			}
			seal.roots[family][ordinal] = subjectRoots[index]
		}
	}
	seal.nodes = builder.nodes
	return nil
}

func guardRouteAdmits(route guardRoute, rank uint32, term keyspace.Term) bool {
	return route.successor.IsBoundary() || route.decision == rank || !route.successor.ResetContains(term)
}

func (seal *guardSeal) validateTransfer(successor causal.Successor) error {
	if err := seal.validateDecision(successor.Decision); err != nil {
		return err
	}
	if successor.IsBoundary() {
		return nil
	}
	if !successor.IsLocal() {
		return errors.New("program/flow/continuation: Successor has an unknown arm")
	}
	if successor.Mu == 0 {
		return nil
	}
	count, countOK := successor.ResetCount()
	if !countOK || count < 0 {
		return errors.New("program/flow/continuation: local route reset is unavailable")
	}
	for index := 0; index < count; index++ {
		decision, ok := successor.ResetAt(index)
		if !ok || !seal.validDecision(decision) {
			return errors.New("program/flow/continuation: local route reset is malformed")
		}
	}
	return nil
}

func (seal *guardSeal) validDecision(term keyspace.Term) bool {
	if term == 0 {
		return true
	}
	_, valid := guardRank(term, seal.counts)
	return valid
}

func (seal *guardSeal) validateDecision(term keyspace.Term) error {
	if !seal.validDecision(term) {
		return errors.New("program/flow/continuation: causal Decision is malformed")
	}
	return nil
}

func (seal *guardSeal) termIndex(term keyspace.Term) (uint32, bool) {
	family, ordinal := keyspace.TermFamily(term), keyspace.TermOrdinal(term)
	if family <= keyspace.FamilyInvalid || family >= keyspace.FamilyCount || ordinal == 0 || ordinal > seal.counts[family] {
		return 0, false
	}
	index := seal.base[family] + ordinal - 1
	return index, uint64(index) < uint64(seal.terms)
}

func (seal *guardSeal) termAt(index uint32) (keyspace.Term, bool) {
	if seal == nil || uint64(index) >= uint64(seal.terms) {
		return 0, false
	}
	for family := keyspace.Family(1); family < keyspace.FamilyCount; family++ {
		base := seal.base[family]
		if index < base || uint64(index-base) >= uint64(seal.counts[family]) {
			continue
		}
		return keyspace.MakeTerm(family, index-base+1), true
	}
	return 0, false
}
