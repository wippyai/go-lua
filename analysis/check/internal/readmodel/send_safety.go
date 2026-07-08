package readmodel

import (
	"fmt"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/domain/placement"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/engine/callboundary"
	factflow "github.com/wippyai/go-lua/analysis/engine/factflow"
	enginestate "github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/ir/cfg"
)

func (r Reader) sendSafetyReports(point cfg.Point, site factflow.CallSiteView, args []CallArgument) []SendSafety {
	if r.result == nil {
		return nil
	}
	outcome, ok := r.result.CallOutcomeAt(point)
	if !ok || len(outcome.NormalReturnFacts.EscapeEvents) == 0 {
		return nil
	}
	seen := make(map[string]struct{})
	var out []SendSafety
	for _, event := range outcome.NormalReturnFacts.EscapeEvents {
		if event.Kind != callboundary.EscapeEventSend {
			continue
		}
		argIndex, ok := sendSafetyArgumentIndex(site, event.Target)
		if !ok {
			continue
		}
		arg, ok := callArgumentByIndex(args, argIndex)
		if !ok {
			continue
		}
		key := fmt.Sprintf("%d:%d:%s", point, argIndex, event.Target.Key())
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, r.sendSafetyReport(point, site, arg, event))
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

func callArgumentByIndex(args []CallArgument, index int) (CallArgument, bool) {
	for _, arg := range args {
		if arg.Index == index {
			return arg, true
		}
	}
	return CallArgument{}, false
}

func (r Reader) sendSafetyReport(point cfg.Point, site factflow.CallSiteView, arg CallArgument, event callboundary.EscapeEventFact) SendSafety {
	source, sourceOK := site.ArgumentSourceAt(arg.Index)
	value := arg.Value
	if sourceOK {
		if before, ok := r.sendSafetySourceValueBefore(point, source); ok {
			value = before
		}
	}
	arg.Value = value
	arg.ValueHash = r.ValueHash(value)
	arg.TypeWithPresence, _ = r.ValueTypeWithPresence(value)

	report := SendSafety{
		Point:     point,
		Argument:  arg,
		Target:    event.Target.Clone(),
		Recursive: event.Recursive,
		Verdict:   SendSafetyUnknown,
		Reason:    "copy fallback: no isolation or immutable proof",
	}
	if sourceOK {
		literal, direct := r.directObjectLiteralSource(source)
		report.DirectObjectLiteral = direct
		if direct {
			if span, ok := literal.Span(); ok {
				report.BirthSpan = sourceSpanFromFactflow(span)
				report.HasBirthSpan = true
			}
		}
		if direct {
			report.GraphHasChildID, report.GraphUnknown = r.objectLiteralGraphHasIdentityChild(point, source, literal)
		}
		if litID, ok := literal.Identity(); ok {
			report.Identity = litID
			report.HasIdentity = true
		}
	}
	if id, ok := identityvalue.ExactID(r.result.Registry(), value); ok {
		report.Identity = id
		report.HasIdentity = true
	}

	factState, factStateOK := r.sendSafetyFactState(point, source)
	if report.HasIdentity && factStateOK {
		report.Placement = factState.ReadPlacement(report.Identity)
		report.HasPlacement = true
		report.Frozen = factState.IsTableFrozen(report.Identity)
	}
	if report.HasIdentity && !report.Frozen {
		if callState, ok := r.result.StateAt(point); ok && callState.IsTableFrozen(report.Identity) {
			report.Frozen = true
		}
	}

	if len(event.Target.Segments) != 0 {
		report.Reason = "copy fallback: send target is a projected member without a whole-graph alias proof"
		return report
	}
	if report.HasIdentity && report.Frozen {
		report.Verdict = SendSafetyProvenImmutable
		report.Reason = "immutable proof: sent exact identity is frozen"
		return report
	}
	if report.DirectObjectLiteral && report.HasIdentity && !report.GraphHasChildID && !report.GraphUnknown {
		report.Verdict = SendSafetyProvenIsolated
		report.Reason = "isolation proof: direct fresh object literal has no retained graph identity"
		return report
	}
	report.Reason = sendSafetyUnknownReason(report)
	return report
}

func (r Reader) sendSafetySourceValueBefore(point cfg.Point, source factflow.ValueSource) (product.Value, bool) {
	if r.result == nil {
		return product.Value{}, false
	}
	if value, ok := r.result.SourceValueBeforeBoundary(point, source); ok {
		return value, true
	}
	return r.result.SourceValueAtBoundary(point, source)
}

func (r Reader) directObjectLiteralSource(source factflow.ValueSource) (factflow.ObjectLiteralView, bool) {
	if r.result == nil || source.Kind != factflow.ValueSourceExpression || !source.HasExpr {
		return factflow.ObjectLiteralView{}, false
	}
	literal, ok := r.result.ObjectLiteralView(source.ExprRef)
	return literal, ok
}

func (r Reader) sendSafetyFactState(point cfg.Point, source factflow.ValueSource) (enginestate.State, bool) {
	if r.result == nil {
		return enginestate.State{}, false
	}
	if source.HasSourcePoint && source.SourcePoint != 0 && source.SourcePoint != point {
		if st, ok := r.result.StateAtBoundary(source.SourcePoint); ok {
			return st, true
		}
	}
	return r.result.StateAt(point)
}

func (r Reader) objectLiteralGraphHasIdentityChild(point cfg.Point, root factflow.ValueSource, literal factflow.ObjectLiteralView) (bool, bool) {
	if r.result == nil {
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

func (r Reader) valueSourceCarriesIdentity(point cfg.Point, root, source factflow.ValueSource) (bool, bool) {
	if source == root {
		return false, false
	}
	if source.Kind == factflow.ValueSourceExpression && source.HasExpr {
		if _, ok := r.result.ExpressionFunction(source.ExprRef); ok {
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
	id, ok := identityvalue.ExactID(r.result.Registry(), value)
	return ok && id != (identity.ID{}), false
}

func sendSafetyUnknownReason(report SendSafety) string {
	if !report.HasIdentity {
		return "copy fallback: sent value has no exact identity proof"
	}
	if report.DirectObjectLiteral {
		if report.GraphHasChildID {
			return "copy fallback: object graph contains another identity that may still be aliased"
		}
		if report.GraphUnknown {
			return "copy fallback: object graph has dynamic or unresolved entries"
		}
	}
	if report.HasPlacement {
		switch report.Placement {
		case placement.Stack:
			return "copy fallback: stack-local path may have aliases across the send"
		case placement.OwnedHeap, placement.SharedHeap, placement.Unknown:
			return fmt.Sprintf("copy fallback: placement is %s", report.Placement)
		}
	}
	return "copy fallback: sent value is not a direct fresh object literal or frozen exact identity"
}
