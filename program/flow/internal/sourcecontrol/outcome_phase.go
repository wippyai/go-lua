package sourcecontrol

import (
	"errors"
	"math/bits"
	"sync"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// outcomePhaseLifecycle owns the one parent-issued extension for exact
// non-Normal Outcome phases.  It retains only sealed paths and the dense
// Outcome-to-path lookup needed while Causal issues endpoint capabilities.
type outcomePhaseLifecycle struct {
	mu     sync.Mutex
	owner  *Result
	phases []OutcomePhase
	byTerm []keyspace.ContentID
	// nonNormal and normalByTerm are seal-local relation guards. They retain
	// no Outcome terms or ordinals, only the dense denominator classification
	// and the exact ordinary BodyTail path for Normal rows while issuance is
	// live.
	nonNormal    []bool
	normalByTerm []keyspace.ContentID
	parentByTerm []keyspace.ContentID
	targetByTerm []keyspace.ContentID
	ownerByTerm  []keyspace.ContentID
	kindByTerm   []kind.OutcomeKind
	state        uint8
}

const (
	outcomePhaseUnissued uint8 = iota
	outcomePhaseIssued
	outcomePhaseConsumed
)

// OutcomePhaseReceipt is a one-shot SourceControl schedule receipt.  Its
// payload is path-only and cannot be used by a foreign Result value.
type OutcomePhaseReceipt struct {
	state *outcomePhaseLifecycle
	owner *Result
}

// OutcomePhase is one parent-issued non-Normal Outcome path in propagation
// order. It has no Body/Outcome ordinal or structural graph coordinate.
type OutcomePhase struct {
	path       keyspace.ContentID
	parentPath keyspace.ContentID
}

func (phase OutcomePhase) VertexPath() (keyspace.ContentID, bool) {
	return phase.path, phase.path.Available()
}
func (phase OutcomePhase) ParentPath() (keyspace.ContentID, bool) {
	if !phase.parentPath.Available() {
		return keyspace.ContentID{}, false
	}
	return phase.parentPath, true
}

// OutcomePhases is the consumed, path-only schedule extension handed to
// recurrence. The path plane cannot be manufactured by downstream callers.
type OutcomePhases struct{ phases []OutcomePhase }

func (proof OutcomePhases) Count() int { return len(proof.phases) }
func (proof OutcomePhases) At(index int) (OutcomePhase, bool) {
	if index < 0 || index >= len(proof.phases) {
		return OutcomePhase{}, false
	}
	return proof.phases[index], true
}

type outcomePhaseCandidate struct {
	term   keyspace.Term
	path   keyspace.ContentID
	parent keyspace.ContentID
}

const vertexOutcomePhaseDomain = "wippy/program/flow/vertex-outcome-phase"

// IssueOutcomePhases consumes the exact certificate Outcome-path receipt and
// issues one distinct phase for every non-Normal Outcome owned by a reachable
// Body. Normal remains the ordinary BodyTail phase; no Body-aligned terminal
// path is retained or issued here.
func (r *Result) IssueOutcomePhases(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	outcomes *outcome.Result,
	pathsReceipt *semanticpath.OutcomePhaseReceipt,
) (*OutcomePhaseReceipt, error) {
	if r == nil || !r.ownerAvailable() || r.outcomePhases == nil || r.outcomePhases.owner != r ||
		!body.Matches(bodies, r.sourceID, r.flowID) ||
		!outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) ||
		sourceView.Identity().ContentID() != r.sourceID || flow.Cold().ContentID() != r.flowID ||
		pathsReceipt == nil || bodies.BodyCount() != len(r.coordinates.bodyOffsets)-1 {
		return nil, errors.New("program/flow/sourcecontrol: outcome-phase owner is unavailable")
	}
	state := r.outcomePhases
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.state != outcomePhaseUnissued {
		return nil, errors.New("program/flow/sourcecontrol: outcome-phase receipt is unavailable")
	}
	paths, pathsOK := pathsReceipt.Consume(r.sourceID, r.flowID, r.staticID, r.moduleID)
	if !pathsOK || paths == nil || paths.Count() != outcomes.Count() {
		return nil, errors.New("program/flow/sourcecontrol: Outcome path receipt is unavailable")
	}

	byTerm := make([]keyspace.ContentID, outcomes.Count()+1)
	nonNormal := make([]bool, outcomes.Count()+1)
	normalByTerm := make([]keyspace.ContentID, outcomes.Count()+1)
	parentByTerm := make([]keyspace.ContentID, outcomes.Count()+1)
	targetByTerm := make([]keyspace.ContentID, outcomes.Count()+1)
	ownerByTerm := make([]keyspace.ContentID, outcomes.Count()+1)
	kindByTerm := make([]kind.OutcomeKind, outcomes.Count()+1)
	candidates := make([]outcomePhaseCandidate, 0, outcomes.Count())
	for index := 0; index < outcomes.Count(); index++ {
		term, termOK := outcomes.At(index)
		if !termOK {
			return nil, errors.New("program/flow/sourcecontrol: Outcome denominator is unavailable")
		}
		owner, outcomeKind, target, rowOK := outcomes.Get(term)
		if !rowOK {
			return nil, errors.New("program/flow/sourcecontrol: Outcome row is unavailable")
		}
		kindByTerm[keyspace.TermOrdinal(term)] = outcomeKind
		ownerByTerm[keyspace.TermOrdinal(term)] = routeTermID(owner)
		if target != 0 {
			targetByTerm[keyspace.TermOrdinal(term)] = routeTermID(target)
		}
		if outcomeKind == kind.OutcomeNormal {
			normal, normalOK := r.BodyTailPhase(owner)
			if !normalOK {
				return nil, errors.New("program/flow/sourcecontrol: Normal BodyTail phase is unavailable")
			}
			normalByTerm[keyspace.TermOrdinal(term)] = normal.path
			continue
		}
		nonNormal[keyspace.TermOrdinal(term)] = true
		entry, entryOK := r.Cursor(owner, 0)
		if !entryOK || !r.Reachable(entry) {
			continue
		}
		semanticPath, pathOK := paths.At(term)
		if !pathOK {
			return nil, errors.New("program/flow/sourcecontrol: Outcome path is unavailable")
		}
		phasePath := vertexPhasePath(vertexOutcomePhaseDomain, semanticPath)
		if !phasePath.Available() {
			return nil, errors.New("program/flow/sourcecontrol: Outcome phase path is unavailable")
		}
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || int(ordinal) >= len(byTerm) || byTerm[ordinal].Available() {
			return nil, errors.New("program/flow/sourcecontrol: Outcome phase denominator is invalid")
		}
		byTerm[ordinal] = phasePath
		parentPath := keyspace.ContentID{}
		if parent, parentOK := outcomes.Propagation(term); parentOK {
			parentSemantic, semanticOK := paths.At(parent)
			if !semanticOK {
				return nil, errors.New("program/flow/sourcecontrol: Outcome parent path is unavailable")
			}
			parentPath = vertexPhasePath(vertexOutcomePhaseDomain, parentSemantic)
			parentByTerm[ordinal] = parentPath
		}
		candidates = append(candidates, outcomePhaseCandidate{term: term, path: phasePath, parent: parentPath})
	}
	radixOutcomePhaseCandidates(candidates)
	phases, ordered := outcomePhaseOrder(candidates)
	if !ordered {
		return nil, errors.New("program/flow/sourcecontrol: Outcome propagation order is unavailable")
	}
	for index := 1; index < len(candidates); index++ {
		if candidates[index-1].path == candidates[index].path {
			return nil, errors.New("program/flow/sourcecontrol: semantic Outcome phase collision")
		}
	}
	state.phases, state.byTerm, state.nonNormal, state.normalByTerm, state.parentByTerm, state.targetByTerm, state.ownerByTerm, state.kindByTerm, state.state = phases, byTerm, nonNormal, normalByTerm, parentByTerm, targetByTerm, ownerByTerm, kindByTerm, outcomePhaseIssued
	return &OutcomePhaseReceipt{state: state, owner: r}, nil
}

// OutcomePhase issues the exact phase for one reachable non-Normal Outcome
// while the parent-issued SourceControl receipt is live.
func (r *Result) OutcomePhase(term keyspace.Term) (PhaseRef, bool) {
	if r == nil || !r.ownerAvailable() || r.outcomePhases == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return PhaseRef{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	state := r.outcomePhases
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.owner != r || state.state != outcomePhaseIssued || ordinal == 0 || uint64(ordinal) >= uint64(len(state.byTerm)) || !state.byTerm[ordinal].Available() {
		return PhaseRef{}, false
	}
	return PhaseRef{result: r, path: state.byTerm[ordinal], class: phaseOutcome, node: noNode}, true
}

// Consume destructively clears the schedule before validating the graph
// owner. A foreign/copy consume therefore burns the exact retry.
func (receipt *OutcomePhaseReceipt) Consume(graph *Result) (OutcomePhases, bool) {
	if receipt == nil || receipt.state == nil {
		return OutcomePhases{}, false
	}
	state := receipt.state
	state.mu.Lock()
	owner, phase := state.owner, state.state
	phases := append([]OutcomePhase(nil), state.phases...)
	state.phases, state.byTerm, state.nonNormal, state.normalByTerm, state.parentByTerm, state.targetByTerm, state.ownerByTerm, state.kindByTerm, state.owner = nil, nil, nil, nil, nil, nil, nil, nil, nil
	state.state = outcomePhaseConsumed
	state.mu.Unlock()
	if owner == nil || phase != outcomePhaseIssued || receipt.owner != owner || graph != owner || !graph.available() {
		return OutcomePhases{}, false
	}
	return OutcomePhases{phases: phases}, true
}

func radixOutcomePhaseCandidates(candidates []outcomePhaseCandidate) {
	if len(candidates) < 2 {
		return
	}
	work := make([]outcomePhaseCandidate, len(candidates))
	for byteIndex := len(keyspace.ContentID{}) - 1; byteIndex >= 0; byteIndex-- {
		var counts [256]int
		for _, item := range candidates {
			counts[item.path[byteIndex]]++
		}
		at := 0
		for index := range counts {
			at, counts[index] = at+counts[index], at
		}
		for _, item := range candidates {
			bucket := item.path[byteIndex]
			work[counts[bucket]] = item
			counts[bucket]++
		}
		copy(candidates, work)
	}
}

// outcomePhaseOrder emits each propagation child before its parent, retaining
// semantic-path radix order for independent chains.
func outcomePhaseOrder(candidates []outcomePhaseCandidate) ([]OutcomePhase, bool) {
	byPath := make(map[keyspace.ContentID]int, len(candidates))
	for index, candidate := range candidates {
		if !candidate.path.Available() {
			return nil, false
		}
		if _, exists := byPath[candidate.path]; exists {
			return nil, false
		}
		byPath[candidate.path] = index
	}
	next := make([]int, len(candidates))
	indegree := make([]int, len(candidates))
	for index := range next {
		next[index] = -1
		parentPath := candidates[index].parent
		if !parentPath.Available() {
			continue
		}
		parentIndex, exists := byPath[parentPath]
		if !exists {
			continue
		}
		next[index] = parentIndex
		indegree[parentIndex]++
	}
	ready := newOutcomeReady(len(candidates))
	for index := range candidates {
		if indegree[index] == 0 {
			ready.add(index)
		}
	}
	ordered := make([]OutcomePhase, 0, len(candidates))
	for {
		current, present := ready.take()
		if !present {
			break
		}
		ordered = append(ordered, OutcomePhase{path: candidates[current].path, parentPath: candidates[current].parent})
		if parent := next[current]; parent >= 0 {
			indegree[parent]--
			if indegree[parent] == 0 {
				ready.add(parent)
			}
		}
	}
	if len(ordered) != len(candidates) {
		return nil, false
	}
	return ordered, true
}

// outcomeReady is a fixed-depth radix priority queue over the already
// semantic-radix-ordered candidate ranks.  Every insert/pop touches at most
// the machine-word summary depth, never scans a ready set, so Kahn remains
// O(Outcome+propagation) while ready ties retain semantic path order.
type outcomeReady struct{ levels [][]uint64 }

func newOutcomeReady(count int) outcomeReady {
	if count <= 0 {
		return outcomeReady{}
	}
	words := (count + 63) / 64
	levels := make([][]uint64, 0, 6)
	for {
		levels = append(levels, make([]uint64, words))
		if words == 1 {
			break
		}
		words = (words + 63) / 64
	}
	return outcomeReady{levels: levels}
}

func (ready outcomeReady) add(rank int) {
	if rank < 0 || len(ready.levels) == 0 {
		return
	}
	index := rank
	for level := range ready.levels {
		word, bit := index>>6, uint(index&63)
		if word >= len(ready.levels[level]) {
			return
		}
		before := ready.levels[level][word]
		ready.levels[level][word] = before | uint64(1)<<bit
		if before != 0 {
			return
		}
		index = word
	}
}

func (ready outcomeReady) take() (int, bool) {
	if len(ready.levels) == 0 || ready.levels[len(ready.levels)-1][0] == 0 {
		return 0, false
	}
	index := 0
	for level := len(ready.levels) - 1; level >= 0; level-- {
		word := ready.levels[level][index]
		if word == 0 {
			return 0, false
		}
		index = index*64 + bits.TrailingZeros64(word)
	}
	ready.remove(index)
	return index, true
}

func (ready outcomeReady) remove(rank int) {
	index := rank
	for level := range ready.levels {
		word, bit := index>>6, uint(index&63)
		if word >= len(ready.levels[level]) {
			return
		}
		ready.levels[level][word] &^= uint64(1) << bit
		if ready.levels[level][word] != 0 {
			return
		}
		index = word
	}
}
