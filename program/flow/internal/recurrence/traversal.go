package recurrence

import (
	"errors"

	"github.com/wippyai/go-lua/program/flow/internal/authored"
	"github.com/wippyai/go-lua/program/flow/internal/body"
	"github.com/wippyai/go-lua/program/flow/internal/containment"
	"github.com/wippyai/go-lua/program/flow/internal/evaluation"
	"github.com/wippyai/go-lua/program/flow/internal/sourcecontrol"
	"github.com/wippyai/go-lua/program/flow/kind"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/source"
)

const noStamp = ^uint32(0)

type semanticEvent struct {
	term      keyspace.Term
	node      uint32
	component uint32
}

type eventTrace struct {
	events []semanticEvent

	labelStamp []uint32
	gotoStamp  []uint32
	loopStart  []uint32
	loopEnd    []uint32
	labelRank  []uint32
	gotoRank   []uint32
}

type eventBuilder struct {
	sourceView source.View
	flow       authored.View
	bodies     *body.Result
	forest     *containment.Result
	graph      *sourcecontrol.Result
	components components

	evaluator   *evaluation.Session
	lexicalRank uint32

	seenBody     []bool
	seenDecision [keyspace.FamilyCount][]bool
	trace        eventTrace
	stack        []walkAction
}

type actionKind uint8

const (
	actionBody actionKind = iota + 1
	actionBranch
	actionLoop
)

type walkAction struct {
	kind actionKind

	body  keyspace.Term
	index int

	term      keyspace.Term
	condition keyspace.Term
	whenTrue  keyspace.Term
	whenFalse keyspace.Term

	child    keyspace.Term
	control  keyspace.Term
	loopKind kind.LoopKind
	phase    uint8
}

func buildEventTrace(
	sourceView source.View,
	flow authored.View,
	bodies *body.Result,
	forest *containment.Result,
	graph *sourcecontrol.Result,
	parts components,
) (eventTrace, error) {
	var empty eventTrace
	if bodies == nil || forest == nil || graph == nil || len(parts.of) != int(graph.NodeCount()) {
		return empty, errors.New("program/flow/recurrence: event owners are unavailable")
	}
	bodyCount := bodies.BodyCount()
	if bodyCount <= 0 {
		return empty, errors.New("program/flow/recurrence: Body denominator is empty")
	}
	evaluator, err := evaluation.New(flow)
	if err != nil {
		return empty, err
	}
	trace := eventTrace{
		labelStamp: make([]uint32, flow.Control().Labels().Count()+1),
		gotoStamp:  make([]uint32, flow.Control().Gotos().Count()+1),
		loopStart:  make([]uint32, flow.Control().Loops().Count()+1),
		loopEnd:    make([]uint32, flow.Control().Loops().Count()+1),
		labelRank:  make([]uint32, flow.Control().Labels().Count()+1),
		gotoRank:   make([]uint32, flow.Control().Gotos().Count()+1),
	}
	fillNoStamp(trace.labelStamp)
	fillNoStamp(trace.gotoStamp)
	fillNoStamp(trace.loopStart)
	fillNoStamp(trace.loopEnd)
	fillNoStamp(trace.labelRank)
	fillNoStamp(trace.gotoRank)

	builder := &eventBuilder{
		sourceView: sourceView,
		flow:       flow,
		bodies:     bodies,
		forest:     forest,
		graph:      graph,
		components: parts,
		evaluator:  evaluator,
		seenBody:   make([]bool, bodyCount+1),
		trace:      trace,
	}
	for _, family := range [...]keyspace.Family{keyspace.FamilySelect, keyspace.FamilyBranch, keyspace.FamilyLoop} {
		count := sourceView.Identity().FamilyCount(family)
		if count < 0 {
			return empty, errors.New("program/flow/recurrence: negative decision denominator")
		}
		builder.seenDecision[family] = make([]bool, count+1)
	}

	// Body parentage, not Body ordinal, chooses structural roots. Function
	// activations are the only independent traversal roots; their authored
	// storage order is harmless because each activation has its own sealed
	// source-control component and local head stream.
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		bodyTerm := keyspace.MakeTerm(keyspace.FamilyBody, uint32(ordinal))
		if _, hasParent := bodies.Parent(bodyTerm); hasParent || builder.seenBody[ordinal] {
			continue
		}
		builder.stack = append(builder.stack, walkAction{kind: actionBody, body: bodyTerm})
		if err := builder.run(); err != nil {
			return empty, err
		}
	}
	functions := flow.Functions()
	for index := 0; index < functions.Count(); index++ {
		function, ok := functions.At(index)
		if !ok {
			return empty, errors.New("program/flow/recurrence: Function activation row is unavailable")
		}
		_, bodyTerm, _, rowOK := functions.Get(function)
		if !rowOK || keyspace.TermFamily(bodyTerm) != keyspace.FamilyBody || keyspace.TermOrdinal(bodyTerm) == 0 ||
			uint64(keyspace.TermOrdinal(bodyTerm)) >= uint64(len(builder.seenBody)) {
			return empty, errors.New("program/flow/recurrence: Function activation Body is invalid")
		}
		if builder.seenBody[keyspace.TermOrdinal(bodyTerm)] {
			continue
		}
		builder.stack = append(builder.stack, walkAction{kind: actionBody, body: bodyTerm})
		if err := builder.run(); err != nil {
			return empty, err
		}
	}
	for ordinal := 1; ordinal <= bodyCount; ordinal++ {
		if !builder.seenBody[ordinal] {
			return empty, errors.New("program/flow/recurrence: Body was not reached from a root or Function activation")
		}
	}
	if err := builder.validateMarkers(); err != nil {
		return empty, err
	}
	return builder.trace, nil
}

func (b *eventBuilder) run() error {
	for len(b.stack) != 0 {
		last := len(b.stack) - 1
		action := b.stack[last]
		b.stack = b.stack[:last]
		switch action.kind {
		case actionBody:
			if err := b.stepBody(action); err != nil {
				return err
			}
		case actionBranch:
			if err := b.stepBranch(action); err != nil {
				return err
			}
		case actionLoop:
			if err := b.stepLoop(action); err != nil {
				return err
			}
		default:
			return errors.New("program/flow/recurrence: unknown traversal action")
		}
	}
	return nil
}

func (b *eventBuilder) stepBody(action walkAction) error {
	ordinal := keyspace.TermOrdinal(action.body)
	if keyspace.TermFamily(action.body) != keyspace.FamilyBody || ordinal == 0 || uint64(ordinal) >= uint64(len(b.seenBody)) {
		return errors.New("program/flow/recurrence: malformed Body traversal")
	}
	if action.index == 0 {
		if b.seenBody[ordinal] {
			return nil
		}
		b.seenBody[ordinal] = true
	} else if !b.seenBody[ordinal] {
		return errors.New("program/flow/recurrence: Body continuation lost its frame")
	}
	order := b.sourceView.Order()
	length, ok := order.BodyLen(action.body)
	if !ok || action.index < 0 || action.index > length {
		return errors.New("program/flow/recurrence: Body source order is unavailable")
	}
	if action.index == length {
		return nil
	}
	term, ok := order.BodyAt(action.body, action.index)
	if !ok {
		return errors.New("program/flow/recurrence: Body source term is unavailable")
	}
	b.stack = append(b.stack, walkAction{kind: actionBody, body: action.body, index: action.index + 1})
	lexicalRank := b.lexicalRank
	if b.lexicalRank == ^uint32(0) {
		return errors.New("program/flow/recurrence: lexical source rank overflows")
	}
	b.lexicalRank++
	switch keyspace.TermFamily(term) {
	case keyspace.FamilyLabel:
		b.markLabel(term, lexicalRank)
	case keyspace.FamilyGoto:
		b.markGoto(term, lexicalRank)
	case keyspace.FamilyBody:
		// Body is a structural child, not a semantic decision. Its direct
		// source position may be absent; graph.Cursor is the sourcecontrol
		// authority used by its eventual decisions.
		b.stack = append(b.stack, walkAction{kind: actionBody, body: term})
	case keyspace.FamilyBranch:
		owner, condition, whenTrue, whenFalse, rowOK := b.flow.Control().Branches().Get(term)
		if !rowOK || owner != action.body {
			return errors.New("program/flow/recurrence: Branch owner disagrees with Source order")
		}
		b.stack = append(b.stack, walkAction{kind: actionBranch, term: term, condition: condition, whenTrue: whenTrue, whenFalse: whenFalse})
	default:
		if keyspace.TermFamily(term) == keyspace.FamilyLoop {
			owner, child, loopKind, control, rowOK := b.flow.Control().Loops().Get(term)
			if !rowOK || owner != action.body {
				return errors.New("program/flow/recurrence: Loop owner disagrees with Source order")
			}
			b.stack = append(b.stack, walkAction{kind: actionLoop, term: term, child: child, control: control, loopKind: loopKind})
			return nil
		}
		switch keyspace.TermFamily(term) {
		case keyspace.FamilyBind, keyspace.FamilyAssign, keyspace.FamilyCall, keyspace.FamilyReturn:
			if err := b.emitSelects(term); err != nil {
				return err
			}
		}
	}
	return nil
}

func (b *eventBuilder) stepBranch(action walkAction) error {
	if action.phase == 0 {
		if !b.active(action.term) {
			// Static or unreachable Branches still own structural Bodies. Walk
			// both arms for coverage, but emit no semantic decision.
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.whenFalse})
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.whenTrue})
			return nil
		}
		if err := b.emitSelects(action.condition); err != nil {
			return err
		}
		if !b.emitDecision(action.term) {
			return errors.New("program/flow/recurrence: Branch decision was not emitted")
		}
		b.stack = append(b.stack, walkAction{kind: actionBranch, phase: 1, whenFalse: action.whenFalse})
		b.stack = append(b.stack, walkAction{kind: actionBody, body: action.whenTrue})
		return nil
	}
	if action.phase == 1 {
		b.stack = append(b.stack, walkAction{kind: actionBody, body: action.whenFalse})
		return nil
	}
	return errors.New("program/flow/recurrence: invalid Branch phase")
}

func (b *eventBuilder) stepLoop(action walkAction) error {
	ordinal := keyspace.TermOrdinal(action.term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(b.trace.loopStart)) {
		return nil
	}
	if !b.active(action.term) {
		if action.phase == 0 {
			// A static/unreachable Loop contributes no event, but its Body is
			// still part of the structural traversal denominator.
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.child})
		}
		return nil
	}
	switch action.loopKind {
	case kind.LoopWhile:
		if action.phase == 0 {
			// The start marker is deliberately before the condition cluster.
			b.trace.loopStart[ordinal] = uint32(len(b.trace.events))
			if err := b.emitSelects(action.control); err != nil {
				return err
			}
			if !b.emitDecision(action.term) {
				return errors.New("program/flow/recurrence: while decision was not emitted")
			}
			b.stack = append(b.stack, walkAction{kind: actionLoop, term: action.term, child: action.child, control: action.control, loopKind: action.loopKind, phase: 1})
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.child})
			return nil
		}
	case kind.LoopNumericFor, kind.LoopGenericFor:
		if action.phase == 0 {
			// Header Selects are one-shot; the range starts immediately at Loop.
			if err := b.emitSelects(action.control); err != nil {
				return err
			}
			b.trace.loopStart[ordinal] = uint32(len(b.trace.events))
			if !b.emitDecision(action.term) {
				return errors.New("program/flow/recurrence: for decision was not emitted")
			}
			b.stack = append(b.stack, walkAction{kind: actionLoop, term: action.term, child: action.child, control: action.control, loopKind: action.loopKind, phase: 1})
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.child})
			return nil
		}
	case kind.LoopRepeat:
		if action.phase == 0 {
			b.trace.loopStart[ordinal] = uint32(len(b.trace.events))
			b.stack = append(b.stack, walkAction{kind: actionLoop, term: action.term, child: action.child, control: action.control, loopKind: action.loopKind, phase: 1})
			b.stack = append(b.stack, walkAction{kind: actionBody, body: action.child})
			return nil
		}
		if action.phase == 1 {
			if err := b.emitSelects(action.control); err != nil {
				return err
			}
			if !b.emitDecision(action.term) {
				return errors.New("program/flow/recurrence: repeat decision was not emitted")
			}
			b.trace.loopEnd[ordinal] = uint32(len(b.trace.events))
			return nil
		}
	default:
		return errors.New("program/flow/recurrence: unsupported Loop kind")
	}
	if action.phase == 1 {
		b.trace.loopEnd[ordinal] = uint32(len(b.trace.events))
		return nil
	}
	return errors.New("program/flow/recurrence: invalid Loop phase")
}

func (b *eventBuilder) active(term keyspace.Term) bool {
	if b.forest.Static(term) {
		return false
	}
	node, ok := decisionCoordinate(b.sourceView, b.flow, b.graph, term)
	return ok && b.graph.Reachable(node)
}

func (b *eventBuilder) emitSelects(container keyspace.Term) error {
	expectedRoot, rootOK := b.sourceView.Index().Root(container)
	if !rootOK || keyspace.TermFamily(expectedRoot) == keyspace.FamilyInvalid {
		return errors.New("program/flow/recurrence: Select container has no source root")
	}
	if err := b.evaluator.Start(container); err != nil {
		return err
	}
	for {
		event, ok, err := b.evaluator.Next()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		root, ok := b.sourceView.Index().Root(event.Select)
		if !ok || root != expectedRoot {
			return errors.New("program/flow/recurrence: evaluation Select escaped source root")
		}
		if !b.active(event.Select) {
			continue
		}
		if !b.emitDecision(event.Select) {
			return errors.New("program/flow/recurrence: Select decision was not emitted")
		}
	}
}

func (b *eventBuilder) emitDecision(term keyspace.Term) bool {
	family := keyspace.TermFamily(term)
	if family != keyspace.FamilySelect && family != keyspace.FamilyBranch && family != keyspace.FamilyLoop {
		return false
	}
	ordinal := keyspace.TermOrdinal(term)
	seen := b.seenDecision[family]
	if ordinal == 0 || uint64(ordinal) >= uint64(len(seen)) || seen[ordinal] || !b.active(term) {
		return false
	}
	node, ok := decisionCoordinate(b.sourceView, b.flow, b.graph, term)
	if !ok || uint64(node) >= uint64(len(b.components.of)) {
		return false
	}
	component := b.components.of[node]
	if component == unassignedComponent {
		return false
	}
	if uint64(len(b.trace.events)) >= uint64(^uint32(0)) {
		return false
	}
	seen[ordinal] = true
	b.trace.events = append(b.trace.events, semanticEvent{term: term, node: node, component: component})
	return true
}

func (b *eventBuilder) markLabel(term keyspace.Term, rank uint32) {
	if !b.active(term) {
		return
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(b.trace.labelStamp)) || b.trace.labelStamp[ordinal] != noStamp {
		return
	}
	b.trace.labelStamp[ordinal] = uint32(len(b.trace.events))
	b.trace.labelRank[ordinal] = rank
}

func (b *eventBuilder) markGoto(term keyspace.Term, rank uint32) {
	if !b.active(term) {
		return
	}
	ordinal := keyspace.TermOrdinal(term)
	if ordinal == 0 || uint64(ordinal) >= uint64(len(b.trace.gotoStamp)) || b.trace.gotoStamp[ordinal] != noStamp {
		return
	}
	b.trace.gotoStamp[ordinal] = uint32(len(b.trace.events))
	b.trace.gotoRank[ordinal] = rank
}

func (b *eventBuilder) validateMarkers() error {
	gotos := b.flow.Control().Gotos()
	for index := 0; index < gotos.Count(); index++ {
		term, ok := gotos.At(index)
		ordinal := keyspace.TermOrdinal(term)
		if !ok || ordinal == 0 || uint64(ordinal) >= uint64(len(b.trace.gotoStamp)) {
			return errors.New("program/flow/recurrence: Goto marker row is invalid")
		}
		if b.active(term) && b.trace.gotoStamp[ordinal] == noStamp {
			return errors.New("program/flow/recurrence: reachable Goto was not traversed")
		}
	}
	labels := b.flow.Control().Labels()
	for index := 0; index < labels.Count(); index++ {
		term, ok := labels.At(index)
		ordinal := keyspace.TermOrdinal(term)
		if !ok || ordinal == 0 || uint64(ordinal) >= uint64(len(b.trace.labelStamp)) {
			return errors.New("program/flow/recurrence: Label marker row is invalid")
		}
		if b.active(term) && b.trace.labelStamp[ordinal] == noStamp {
			return errors.New("program/flow/recurrence: reachable Label was not traversed")
		}
	}
	return nil
}

func fillNoStamp(values []uint32) {
	for index := range values {
		values[index] = noStamp
	}
}
