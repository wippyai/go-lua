package body

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/engine/state/escapeevent"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

// SendSafetyVerdict classifies whether a call-boundary send payload is
// eligible for zero-copy transfer from solved ownership facts.
type SendSafetyVerdict uint8

const (
	SendSafetyUnknown SendSafetyVerdict = iota
	SendSafetyProvenIsolated
	SendSafetyProvenImmutable
	SendSafetyRefutedEscaped
)

// SendSafetyOccurrence is the body-owned semantic candidate for one send
// payload admission check. Readmodel consumers attach call-argument display
// fields and expose the public API shape.
type SendSafetyOccurrence struct {
	Point               cfg.Point
	ArgumentIndex       int
	ArgumentValue       product.Value
	HasArgumentValue    bool
	Target              pathdom.Path
	Recursive           bool
	Verdict             SendSafetyVerdict
	Reason              string
	Identity            identity.ID
	HasIdentity         bool
	BirthSpan           SourceSpan
	HasBirthSpan        bool
	Placement           placement.Value
	HasPlacement        bool
	Frozen              bool
	DirectObjectLiteral bool
	GraphHasChildID     bool
	GraphUnknown        bool
}

// SendSafetyOccurrences returns send/spawn payload admission candidates for
// call-boundary escape facts at point.
func (r *Result) SendSafetyOccurrences(point cfg.Point) []SendSafetyOccurrence {
	if r == nil {
		return nil
	}
	outcome, ok := r.CallOutcomeAt(point)
	if !ok || len(outcome.NormalReturnFacts.EscapeEvents) == 0 {
		return nil
	}
	site, ok := r.CallSiteView(point)
	if !ok {
		return nil
	}
	seen := make(map[string]struct{})
	var out []SendSafetyOccurrence
	for _, event := range outcome.NormalReturnFacts.EscapeEvents {
		if event.Kind != callboundary.EscapeEventSend {
			continue
		}
		argIndex, ok := sendSafetyArgumentIndex(site, event.Target)
		if !ok {
			continue
		}
		if markSeen(seen, fmt.Sprintf("%d:%d:%s", point, argIndex, event.Target.Key())) {
			continue
		}
		out = append(out, r.sendSafetyOccurrence(point, site, argIndex, event))
	}
	return out
}

func sendSafetyArgumentIndex(site factflow.CallSiteView, target pathdom.Path) (int, bool) {
	if !target.IsPlaceholder() {
		return 0, false
	}
	index := target.PlaceholderIndex()
	if index < 0 {
		return 0, false
	}
	if _, hasReceiver := site.ReceiverPath(); hasReceiver {
		if index == 0 {
			return 0, false
		}
		return index - 1, true
	}
	return index, true
}

func (r *Result) sendSafetyOccurrence(point cfg.Point, site factflow.CallSiteView, argIndex int, event callboundary.EscapeEventFact) SendSafetyOccurrence {
	source, sourceOK := site.ArgumentSourceAt(argIndex)
	value, valueOK := product.Value{}, false
	if sourceOK {
		value, valueOK = r.sendSafetySourceValueBefore(point, source)
	}

	occ := SendSafetyOccurrence{
		Point:            point,
		ArgumentIndex:    argIndex,
		ArgumentValue:    value,
		HasArgumentValue: valueOK,
		Target:           event.Target.Clone(),
		Recursive:        event.Recursive,
	}
	if sourceOK {
		literal, direct := r.directObjectLiteralSource(source)
		occ.DirectObjectLiteral = direct
		if direct {
			if span, ok := literal.Span(); ok {
				occ.BirthSpan = sourceSpanFromFactflow(span)
				occ.HasBirthSpan = true
			}
			occ.GraphHasChildID, occ.GraphUnknown = r.objectLiteralGraphHasIdentityChild(point, source, literal)
		}
		if litID, ok := literal.Identity(); ok {
			occ.Identity = litID
			occ.HasIdentity = true
		}
	}
	if valueOK {
		if id, ok := identityvalue.ExactID(r.Registry(), value); ok {
			occ.Identity = id
			occ.HasIdentity = true
		}
	}

	factState, factStateOK := r.sendSafetyFactState(point)
	if occ.HasIdentity && factStateOK {
		occ.Placement = factState.ReadPlacement(occ.Identity)
		occ.HasPlacement = true
		occ.Frozen = r.sendSafetyFrozenAtInput(point, occ.Identity, factState)
	}

	if len(event.Target.Segments) != 0 {
		occ.Reason = "copy fallback: send target is a projected member without a whole-graph alias proof"
		return occ
	}
	if occ.HasIdentity && occ.Frozen {
		occ.Verdict = SendSafetyProvenImmutable
		occ.Reason = "immutable proof: sent exact identity is frozen"
		return occ
	}
	if occ.DirectObjectLiteral && occ.HasIdentity && !occ.GraphHasChildID && !occ.GraphUnknown {
		occ.Verdict = SendSafetyProvenIsolated
		occ.Reason = "isolation proof: direct fresh object literal has no retained graph identity"
		return occ
	}
	if factStateOK && r.sendSafetyHasPriorEscape(point, source, factState) {
		occ.Verdict = SendSafetyRefutedEscaped
		occ.Reason = "escape proof: payload has already crossed a retaining boundary"
		return occ
	}
	occ.Reason = sendSafetyUnknownReason(occ)
	return occ
}

func (r *Result) sendSafetySourceValueBefore(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r == nil {
		return product.Value{}, false
	}
	if value, ok := r.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.SourceValueAtBoundary(point, source)
}

func (r *Result) directObjectLiteralSource(source factflow.ValueSource) (factflow.ObjectLiteralView, bool) {
	if r == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ObjectLiteralView{}, false
	}
	return r.ObjectLiteralView(source.ExprRef)
}

func (r *Result) sendSafetyFactState(point cfg.Point) (state.State, bool) {
	if r == nil {
		return state.State{}, false
	}
	return r.StateAt(point)
}

// sendSafetyFrozenAtInput accepts only must-frozen evidence that holds before
// the send. The state lane is authoritative when populated. Some solved
// boundary plans retain a frozen-table normal-return fact per call without
// materializing that lane at its successor; a dominating normal-return fact
// for the same exact identity is equally a must fact at this point. This never
// reads the send boundary output, so the send cannot prove itself immutable.
func (r *Result) sendSafetyFrozenAtInput(point cfg.Point, id identity.ID, in state.State) bool {
	if r == nil || id == (identity.ID{}) {
		return false
	}
	if in.IsTableFrozen(id) {
		return true
	}
	graph := r.Graph()
	if graph == nil {
		return false
	}
	for _, candidate := range graph.RPO() {
		if candidate == point || !r.PointDominates(candidate, point) {
			continue
		}
		outcome, ok := r.CallOutcomeAt(candidate)
		if !ok || len(outcome.NormalReturnFacts.FrozenTables) == 0 {
			continue
		}
		bindings := r.callGuardCallBindingsAt(candidate)
		for _, fact := range outcome.NormalReturnFacts.FrozenTables {
			target, ok := fact.Target.Substitute(bindings)
			if !ok || target.IsEmpty() {
				continue
			}
			value, ok := r.PathValueBeforeBoundary(candidate, target)
			if !ok {
				continue
			}
			frozenID, exact := identityvalue.ExactID(r.Registry(), value)
			if exact && frozenID == id {
				return true
			}
		}
	}
	return false
}

// sendSafetyHasPriorEscape accepts only must escape facts already present at
// the call input. The current send is produced by the call outcome and is not
// part of this state, so it cannot refute every boundary by itself.
func (r *Result) sendSafetyHasPriorEscape(point cfg.Point, source factflow.ValueSource, factState state.State) bool {
	p, ok := r.valueSourcePath(source)
	if !ok || r.KeySpace() == nil {
		return false
	}
	target, ok := valueSourcePathStateKey(r.visibility, point, p)
	if !ok {
		return false
	}
	keys := r.KeySpace()
	targetKey, ok := keys.FromStateKey(target.PathKey())
	if !ok {
		return false
	}
	for _, event := range factState.EscapeEventsSnapshot().Facts {
		// Borrow is not a retained alias. Retain and stronger events are
		// must facts that prevent treating this value as iso.
		if event.Kind < escapeevent.KindRetain {
			continue
		}
		eventKey, ok := keys.FromStateKey(event.Target.PathKey())
		if !ok {
			continue
		}
		if targetKey == eventKey || (event.Recursive && keys.HasPrefix(targetKey, eventKey)) {
			return true
		}
	}
	return false
}

func (r *Result) objectLiteralGraphHasIdentityChild(point cfg.Point, root factflow.ValueSource, literal factflow.ObjectLiteralView) (bool, bool) {
	if r == nil {
		return false, true
	}
	hasChild := false
	unknown := !literal.StaticStringKeysComplete()
	literal.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		child, childUnknown := r.valueSourceCarriesIdentity(point, root, entry.Source())
		if child {
			hasChild = true
			return false
		}
		if childUnknown {
			unknown = true
		}
		return true
	})
	if source, ok := literal.ListElementSource(); ok {
		child, childUnknown := r.valueSourceCarriesIdentity(point, root, source)
		if child {
			hasChild = true
		}
		if childUnknown {
			unknown = true
		}
	}
	return hasChild, unknown
}

func (r *Result) valueSourceCarriesIdentity(point cfg.Point, root, source factflow.ValueSource) (bool, bool) {
	if source == root {
		return false, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, ok := r.ExpressionFunction(source.ExprRef); ok {
			return true, false
		}
	}
	value, ok := r.sendSafetySourceValueBefore(point, source)
	if !ok {
		switch source.Kind {
		case factflow.ValueSourceLiteral, factflow.ValueSourceNil:
			return false, false
		default:
			return false, true
		}
	}
	id, ok := identityvalue.ExactID(r.Registry(), value)
	return ok && id != (identity.ID{}), false
}

func sendSafetyUnknownReason(occ SendSafetyOccurrence) string {
	if !occ.HasIdentity {
		return "copy fallback: sent value has no exact identity proof"
	}
	if occ.DirectObjectLiteral {
		if occ.GraphHasChildID {
			return "copy fallback: object graph contains another identity that may still be aliased"
		}
		if occ.GraphUnknown {
			return "copy fallback: object graph has dynamic or unresolved entries"
		}
	}
	if occ.HasPlacement {
		switch occ.Placement {
		case placement.Stack:
			return "copy fallback: stack-local path may have aliases across the send"
		case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
			return fmt.Sprintf("copy fallback: placement is %s", occ.Placement)
		}
	}
	return "copy fallback: sent value is not a direct fresh object literal or frozen exact identity"
}
