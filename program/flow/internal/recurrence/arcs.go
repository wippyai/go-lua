package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

// arcWork is Seal-local recurrence evidence aligned with sourcecontrol Arc
// order. first/past are global semantic-event boundaries until stream
// materialization converts them to the owning head's local ranks.
type arcWork struct {
	head      keyspace.Term
	component uint32
	first     uint32
	past      uint32
	recurrent bool
}

func classifyArcs(
	sourceView source.View,
	flow authored.View,
	graph *sourcecontrol.Result,
	parts components,
	heads []keyspace.Term,
	trace eventTrace,
	counts [keyspace.FamilyCount]uint32,
) ([]arcWork, error) {
	work := make([]arcWork, graph.ArcCount())
	loops := flow.Control().Loops()
	for index := 0; index < graph.ArcCount(); index++ {
		arc, ok := graph.ArcAt(index)
		if !ok || arc.From >= graph.NodeCount() || arc.To >= graph.NodeCount() {
			return nil, errors.New("program/flow/recurrence: malformed sourcecontrol Arc")
		}
		if !validExisting(arc.Source, counts) || !validExisting(arc.Target, counts) ||
			(arc.Decision != 0 && !validExisting(arc.Decision, counts)) || (arc.Decision == 0 && arc.Truth) ||
			(arc.Decision != 0 && keyspace.TermFamily(arc.Decision) != keyspace.FamilyBranch && keyspace.TermFamily(arc.Decision) != keyspace.FamilyLoop) {
			return nil, errors.New("program/flow/recurrence: sourcecontrol Arc identity is invalid")
		}
		if !graph.Reachable(arc.From) || !graph.Reachable(arc.To) || parts.of[arc.From] != parts.of[arc.To] {
			continue
		}
		component := parts.of[arc.From]
		if component == unassignedComponent || !parts.cyclic[component] || heads[component] == 0 {
			continue
		}
		first, past, recurrent, err := classifyArc(sourceView, flow, graph, trace, loops, arc, counts)
		if err != nil {
			return nil, err
		}
		if !recurrent {
			continue
		}
		work[index] = arcWork{head: heads[component], component: component, first: first, past: past, recurrent: true}
	}
	return work, nil
}

func classifyArc(
	sourceView source.View,
	flow authored.View,
	graph *sourcecontrol.Result,
	trace eventTrace,
	loops authored.Loops,
	arc sourcecontrol.Arc,
	counts [keyspace.FamilyCount]uint32,
) (uint32, uint32, bool, error) {
	loopFor := func(term keyspace.Term) (keyspace.Term, keyspace.Term, kind.LoopKind, bool) {
		if keyspace.TermFamily(term) != keyspace.FamilyLoop {
			return 0, 0, 0, false
		}
		_, child, loopKind, _, ok := loops.Get(term)
		return term, child, loopKind, ok
	}
	if keyspace.TermFamily(arc.Source) == keyspace.FamilyBody && keyspace.TermFamily(arc.Target) == keyspace.FamilyLoop && arc.Decision == 0 {
		loop, child, loopKind, ok := loopFor(arc.Target)
		if !ok || !validExisting(child, counts) || arc.Source != child {
			return 0, 0, false, nil
		}
		childTail, tailOK := graph.Tail(child)
		loopNode, loopOK := decisionCoordinate(sourceView, flow, graph, loop)
		if !tailOK || !loopOK {
			return 0, 0, false, errors.New("program/flow/recurrence: Loop feedback coordinate is unavailable")
		}
		if arc.From != childTail {
			return 0, 0, false, errors.New("program/flow/recurrence: Loop feedback source coordinate disagrees")
		}
		switch loopKind {
		case kind.LoopWhile:
			if arc.To != loopNode {
				return 0, 0, false, errors.New("program/flow/recurrence: While feedback target coordinate disagrees")
			}
			return loopRange(trace, loop, true)
		case kind.LoopNumericFor, kind.LoopGenericFor:
			if arc.To != loopNode {
				return 0, 0, false, errors.New("program/flow/recurrence: For feedback target coordinate disagrees")
			}
			return loopRange(trace, loop, true)
		default:
			return 0, 0, false, nil
		}
	}
	if keyspace.TermFamily(arc.Source) == keyspace.FamilyLoop && keyspace.TermFamily(arc.Target) == keyspace.FamilyBody &&
		arc.Decision == arc.Source && !arc.Truth {
		loop, child, loopKind, ok := loopFor(arc.Source)
		if !ok || loopKind != kind.LoopRepeat || !validExisting(child, counts) || arc.Target != child {
			return 0, 0, false, nil
		}
		decision, decisionOK := decisionCoordinate(sourceView, flow, graph, loop)
		start, startOK := graph.Cursor(child, 0)
		if !decisionOK || !startOK {
			return 0, 0, false, errors.New("program/flow/recurrence: Repeat feedback coordinate is unavailable")
		}
		if arc.From != decision || arc.To != start {
			return 0, 0, false, errors.New("program/flow/recurrence: Repeat feedback endpoint disagrees")
		}
		return loopRange(trace, loop, true)
	}
	if keyspace.TermFamily(arc.Source) == keyspace.FamilyGoto && keyspace.TermFamily(arc.Target) == keyspace.FamilyLabel && arc.Decision == 0 && !arc.Truth {
		_, target, rowOK := flow.Control().Gotos().Get(arc.Source)
		if !rowOK || target != arc.Target {
			return 0, 0, false, errors.New("program/flow/recurrence: Goto target disagrees with authored control")
		}
		backward, ok := backwardGoto(trace, arc.Source, arc.Target)
		if !ok || !backward {
			return 0, 0, false, nil
		}
		gotoNode, gotoOK := graph.Coordinate(sourceView, arc.Source)
		labelNode, labelOK := graph.Coordinate(sourceView, arc.Target)
		if !gotoOK || !labelOK {
			return 0, 0, false, errors.New("program/flow/recurrence: Goto/Label coordinate is unavailable")
		}
		if arc.From != gotoNode || arc.To != labelNode {
			return 0, 0, false, errors.New("program/flow/recurrence: Goto/Label Arc endpoint disagrees with Source")
		}
		gotoOrdinal := keyspace.TermOrdinal(arc.Source)
		labelOrdinal := keyspace.TermOrdinal(arc.Target)
		if uint64(gotoOrdinal) >= uint64(len(trace.gotoStamp)) || uint64(labelOrdinal) >= uint64(len(trace.labelStamp)) {
			return 0, 0, false, nil
		}
		first, past := trace.labelStamp[labelOrdinal], trace.gotoStamp[gotoOrdinal]
		if first == noStamp || past == noStamp || first > past {
			return 0, 0, false, errors.New("program/flow/recurrence: backward Goto markers are incomplete")
		}
		return first, past, true, nil
	}
	return 0, 0, false, nil
}

func loopRange(trace eventTrace, loop keyspace.Term, requireRange bool) (uint32, uint32, bool, error) {
	ordinal := keyspace.TermOrdinal(loop)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(trace.loopStart)) ||
		trace.loopStart[ordinal] == noStamp || trace.loopEnd[ordinal] == noStamp || trace.loopStart[ordinal] > trace.loopEnd[ordinal] {
		if requireRange {
			return 0, 0, false, errors.New("program/flow/recurrence: loop event boundaries are incomplete")
		}
		return 0, 0, false, nil
	}
	return trace.loopStart[ordinal], trace.loopEnd[ordinal], true, nil
}

func backwardGoto(trace eventTrace, gotoTerm, labelTerm keyspace.Term) (bool, bool) {
	// Source position is only comparable within one Body. A nested or
	// outward jump needs one lexical root order shared by both marker kinds;
	// event stamps are semantic boundaries and cannot distinguish an empty
	// forward jump from an empty backward jump. This is only the authored
	// source-order classification used to select an SCC reset; it is not a
	// termination, divergence, or convergence judgment.
	gotoOrdinal, labelOrdinal := keyspace.TermOrdinal(gotoTerm), keyspace.TermOrdinal(labelTerm)
	if uint64(gotoOrdinal) >= uint64(len(trace.gotoRank)) || uint64(labelOrdinal) >= uint64(len(trace.labelRank)) {
		return false, false
	}
	if trace.gotoRank[gotoOrdinal] == noStamp || trace.labelRank[labelOrdinal] == noStamp {
		return false, false
	}
	return trace.labelRank[labelOrdinal] < trace.gotoRank[gotoOrdinal], true
}

func requireCyclicWitness(parts components, heads []keyspace.Term, work []arcWork) error {
	seen := make([]bool, len(parts.sizes))
	for _, item := range work {
		if item.recurrent && item.component < uint32(len(seen)) {
			seen[item.component] = true
		}
	}
	for component, cyclic := range parts.cyclic {
		if cyclic && heads[component] != 0 && !seen[component] {
			return errors.New("program/flow/recurrence: cyclic component has no recognized recurrence Arc")
		}
	}
	return nil
}
