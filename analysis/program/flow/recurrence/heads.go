package recurrence

import (
	"sort"

	"github.com/wippyai/go-lua/analysis/program/flow/authored"
	"github.com/wippyai/go-lua/analysis/program/flow/body"
	"github.com/wippyai/go-lua/analysis/program/flow/containment"
	"github.com/wippyai/go-lua/analysis/program/flow/kind"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

// headLayout retains only the dense authored-loop lookup needed while the
// recurrence seal is live.  byLoop is not a second topology: it selects the
// canonical Mu head for a structural loop feedback Arc. aliases are local
// views over the primary component stream and are discarded at publication.
type headLayout struct {
	byLoop  []keyspace.Term
	aliases []headAlias
}

type loopHeadCandidate struct {
	term      keyspace.Term
	owner     keyspace.Term
	body      keyspace.Term
	component uint32
	start     uint32
	past      uint32
}

// deriveNestedHeadLayout gives nested while feedback its own Mu head while
// retaining the existing primary SCC component directory.  A component's
// primary stream still contains the complete semantic event interval; each
// additional head is a one-decision view over that stream.  This is needed
// because lexical nested loops are one source-control SCC but have distinct
// final feedback reset owners.
func deriveNestedHeadLayout(
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	parts components,
	heads []keyspace.Term,
	trace eventTrace,
	counts [keyspace.FamilyCount]uint32,
) headLayout {
	layout := headLayout{byLoop: make([]keyspace.Term, int(counts[keyspace.FamilyLoop])+1)}
	if bodies == nil || forest == nil {
		return layout
	}
	loops := flow.Control().Loops()
	byComponent := make([][]loopHeadCandidate, len(parts.sizes))
	for index := 0; index < loops.Count(); index++ {
		term, ok := loops.At(index)
		if !ok || !validExisting(term, counts) || forest.Static(term) {
			continue
		}
		owner, child, loopKind, _, rowOK := loops.Get(term)
		if !rowOK || loopKind != kind.LoopWhile || !validExisting(owner, counts) || !validExisting(child, counts) {
			continue
		}
		// The event trace already proved reachability and records the exact
		// source-control node for every active decision. Reusing that issued
		// event avoids reopening an occurrence-to-coordinate authority here.
		var node uint32
		nodeOK := false
		for _, event := range trace.events {
			if event.term == term {
				node, nodeOK = event.node, true
				break
			}
		}
		if !nodeOK || uint64(node) >= uint64(len(parts.of)) {
			continue
		}
		component := parts.of[node]
		if component == unassignedComponent || uint64(component) >= uint64(len(byComponent)) || heads[component] == 0 {
			continue
		}
		ordinal := keyspace.TermOrdinal(term)
		if ordinal == 0 || uint64(ordinal) >= uint64(len(trace.loopStart)) || trace.loopStart[ordinal] == noStamp || trace.loopEnd[ordinal] == noStamp || trace.loopEnd[ordinal] < trace.loopStart[ordinal] {
			continue
		}
		byComponent[component] = append(byComponent[component], loopHeadCandidate{
			term: term, owner: owner, body: child, component: component,
			start: trace.loopStart[ordinal], past: trace.loopEnd[ordinal],
		})
	}

	for component, candidates := range byComponent {
		if len(candidates) < 2 || !nestedLoopCandidates(bodies, candidates) {
			continue
		}
		// The primary component stream must already be headed by one of the
		// nested loops before a local loop layout can be projected. A label-led
		// irreducible component has no one-to-one loop-head assignment; leave it
		// on the ordinary component path instead of manufacturing a partial one.
		primaryCandidate := false
		for _, candidate := range candidates {
			if candidate.term == heads[component] {
				primaryCandidate = true
				break
			}
		}
		if !primaryCandidate {
			continue
		}
		// The authored Control view is dense by Loop identity, but sort the
		// local copy explicitly so this layout remains canonical if a caller
		// supplies an equivalent view with a different row order.
		sort.Slice(candidates, func(left, right int) bool { return candidates[left].term < candidates[right].term })
		candidateHeads := make([]keyspace.Term, 0, len(candidates)+1)
		candidateHeads = append(candidateHeads, heads[component])
		for _, candidate := range candidates {
			if candidate.term != heads[component] {
				candidateHeads = append(candidateHeads, candidate.term)
			}
		}
		// Wider event intervals are enclosing loops. Pairing those intervals
		// with the canonical head order gives deterministic outer-to-inner Mu
		// identities even when Source allocates nested Loop terms postorder.
		sort.SliceStable(candidates, func(left, right int) bool {
			leftWidth := candidates[left].past - candidates[left].start
			rightWidth := candidates[right].past - candidates[right].start
			if leftWidth != rightWidth {
				return leftWidth > rightWidth
			}
			if candidates[left].start != candidates[right].start {
				return candidates[left].start < candidates[right].start
			}
			return candidates[left].term < candidates[right].term
		})
		for index, candidate := range candidates {
			if index >= len(candidateHeads) {
				break
			}
			head := candidateHeads[index]
			ordinal := keyspace.TermOrdinal(candidate.term)
			if uint64(ordinal) < uint64(len(layout.byLoop)) {
				layout.byLoop[ordinal] = head
			}
			if head != heads[component] {
				layout.aliases = append(layout.aliases, headAlias{head: head, component: uint32(component)})
			}
		}
	}
	return layout
}

func nestedLoopCandidates(bodies *body.Result, candidates []loopHeadCandidate) bool {
	for left := range candidates {
		for right := range candidates {
			if left == right {
				continue
			}
			// An inner loop is authored in the outer loop's child Body. The
			// transitive query also handles a Body containing a nested branch
			// before the inner loop.
			if bodies.Contains(candidates[left].body, candidates[right].owner) || bodies.Contains(candidates[right].body, candidates[left].owner) {
				return true
			}
		}
	}
	return false
}
