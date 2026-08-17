package sourcecontrol

import (
	"errors"
	"math/bits"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/body"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/outcome"
	"github.com/wippyai/go-lua/analysis/program/flow/internal/semanticpath"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

// OutcomePhases is the immutable SourceControl-owned schedule for exact
// reachable non-Normal Outcome phases. It retains only sealed paths and the
// dense Outcome-to-path relations needed by route planning and recurrence.
// The owner fence is intentionally the SourceControl Result itself: callers
// can retain the view, but cannot use it with a different sealed graph.
type OutcomePhases struct {
	owner  *Result
	phases []OutcomePhase
	byTerm []identity.ContentID
	// nonNormal and normalByTerm are seal-local relation guards. They retain
	// no Outcome terms or ordinals, only the dense denominator classification
	// and the exact ordinary BodyTail path for Normal rows.
	nonNormal    []bool
	normalByTerm []identity.ContentID
	parentByTerm []identity.ContentID
	targetByTerm []identity.ContentID
	ownerByTerm  []identity.ContentID
	kindByTerm   []kind.OutcomeKind
}

// OutcomePhase is one SourceControl-owned non-Normal Outcome path in propagation
// order. It has no Body/Outcome ordinal or structural graph coordinate.
type OutcomePhase struct {
	path       identity.ContentID
	parentPath identity.ContentID
}

func (phase OutcomePhase) VertexPath() (identity.ContentID, bool) {
	return phase.path, phase.path.Available()
}
func (phase OutcomePhase) ParentPath() (identity.ContentID, bool) {
	if !phase.parentPath.Available() {
		return identity.ContentID{}, false
	}
	return phase.parentPath, true
}

func (proof OutcomePhases) Count() int { return len(proof.phases) }
func (proof OutcomePhases) At(index int) (OutcomePhase, bool) {
	if index < 0 || index >= len(proof.phases) {
		return OutcomePhase{}, false
	}
	return proof.phases[index], true
}

type outcomePhaseCandidate struct {
	term   keyspace.Term
	path   identity.ContentID
	parent identity.ContentID
}

const vertexOutcomePhaseDomain = "wippy/program/flow/vertex-outcome-phase"

// BuildOutcomePhases reads the exact certificate Outcome-path view and builds
// one immutable phase for every non-Normal Outcome owned by a reachable Body.
// Normal remains the ordinary BodyTail phase; no Body-aligned terminal path is
// retained or issued here. The view is installed on r exactly once and is
// subsequently read by direct queries and recurrence without a lifecycle
// transaction.
func (r *Result) BuildOutcomePhases(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	outcomes *outcome.Result,
	paths *semanticpath.OutcomePhasePaths,
) (*OutcomePhases, error) {
	if r == nil || !r.ownerAvailable() ||
		!body.Matches(bodies, r.sourceID, r.flowID) ||
		!outcome.Matches(outcomes, r.sourceID, r.flowID, r.staticID, r.moduleID) ||
		sourceView.Identity().ContentID() != r.sourceID || flow.Cold().ContentID() != r.flowID ||
		paths == nil || !paths.Matches(r.sourceID, r.flowID, r.staticID, r.moduleID) || bodies.BodyCount() != len(r.coordinates.bodyOffsets)-1 {
		return nil, errors.New("program/flow/sourcecontrol: outcome-phase owner is unavailable")
	}
	if r.outcomePhases != nil {
		return nil, errors.New("program/flow/sourcecontrol: outcome-phase view is already built")
	}
	if paths.Count() != outcomes.Count() {
		return nil, errors.New("program/flow/sourcecontrol: Outcome path view is unavailable")
	}

	byTerm := make([]identity.ContentID, outcomes.Count()+1)
	nonNormal := make([]bool, outcomes.Count()+1)
	normalByTerm := make([]identity.ContentID, outcomes.Count()+1)
	parentByTerm := make([]identity.ContentID, outcomes.Count()+1)
	targetByTerm := make([]identity.ContentID, outcomes.Count()+1)
	ownerByTerm := make([]identity.ContentID, outcomes.Count()+1)
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
		parentPath := identity.ContentID{}
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
	identity.SortByContentID(candidates, outcomePhaseCandidatePath)
	phases, ordered := outcomePhaseOrder(candidates)
	if !ordered {
		return nil, errors.New("program/flow/sourcecontrol: Outcome propagation order is unavailable")
	}
	for index := 1; index < len(candidates); index++ {
		if candidates[index-1].path == candidates[index].path {
			return nil, errors.New("program/flow/sourcecontrol: semantic Outcome phase collision")
		}
	}
	view := &OutcomePhases{owner: r, phases: phases, byTerm: byTerm, nonNormal: nonNormal, normalByTerm: normalByTerm, parentByTerm: parentByTerm, targetByTerm: targetByTerm, ownerByTerm: ownerByTerm, kindByTerm: kindByTerm}
	r.outcomePhases = view
	return view, nil
}

// OutcomePhase resolves the exact phase for one reachable non-Normal Outcome
// from the immutable SourceControl-owned schedule.
func (r *Result) OutcomePhase(term keyspace.Term) (PhaseRef, bool) {
	if r == nil || !r.ownerAvailable() || r.outcomePhases == nil || keyspace.TermFamily(term) != keyspace.FamilyOutcome {
		return PhaseRef{}, false
	}
	ordinal := keyspace.TermOrdinal(term)
	view := r.outcomePhases
	if view.owner != r || ordinal == 0 || uint64(ordinal) >= uint64(len(view.byTerm)) || !view.byTerm[ordinal].Available() {
		return PhaseRef{}, false
	}
	return PhaseRef{result: r, path: view.byTerm[ordinal], class: phaseOutcome, node: noNode}, true
}

// Matches reports whether view was built by the exact SourceControl Result.
// It is the only admissibility check needed when the immutable schedule is
// handed across the causal/recurrence boundary.
func (view *OutcomePhases) Matches(graph *Result) bool {
	return view != nil && graph != nil && view.owner == graph && graph.outcomePhases == view && graph.available()
}

// outcomePhaseOrder emits each propagation child before its parent, retaining
// semantic-path radix order for independent chains.
func outcomePhaseOrder(candidates []outcomePhaseCandidate) ([]OutcomePhase, bool) {
	byPath := make(map[identity.ContentID]int, len(candidates))
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

func outcomePhaseCandidatePath(item outcomePhaseCandidate) identity.ContentID { return item.path }
