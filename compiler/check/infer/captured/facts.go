package captured

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/compiler/check/domain/functionsymbols"
	interprocdomain "github.com/wippyai/go-lua/compiler/check/domain/interproc"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/typ"
	"github.com/wippyai/go-lua/types/typ/unwrap"
)

// AssignedValueType observes a captured assignment's lowered flow-source value.
type AssignedValueType func(cfg.Point, constraint.Path, typ.Type, flow.AssignmentSource) typ.Type

// liftCarrierType lifts a producer-computed container mutation type onto the
// interned carrier at admission. A nil type lifts to the zero AbstractValue so
// an absent key slot stays absent across the round-trip.
func liftCarrierType(t typ.Type) product.AbstractValue {
	if t == nil {
		return product.AbstractValue{}
	}
	return product.FromType(t)
}

// PromotedFieldSet identifies field slots proven present at a capture boundary.
type PromotedFieldSet map[interprocdomain.FieldKey]bool

// PromotedFields maps captured symbols to their proven-present field slots.
type PromotedFields map[cfg.SymbolID]PromotedFieldSet

// FieldFactsFromEvidence reduces captured field assignment evidence into
// canonical captured-field facts.
func FieldFactsFromEvidence(evidence []api.CapturedFieldEvidence, observe AssignedValueType) map[cfg.SymbolID]interprocdomain.FieldValues {
	if len(evidence) == 0 {
		return nil
	}
	fields := make(map[cfg.SymbolID]interprocdomain.FieldValues)
	for _, ev := range evidence {
		if ev.Target == 0 || ev.Field == "" {
			continue
		}
		fieldType := ev.ValueType
		if observe != nil {
			if t := observe(ev.Point, ev.TargetPath, ev.ValueType, ev.ValueSource); t != nil {
				fieldType = t
			}
		}
		if fieldType == nil {
			fieldType = typ.Unknown
		}
		field, fieldType, ok := capturedFieldFactValue(ev, fieldType)
		if !ok {
			continue
		}
		if fields[ev.Target] == nil {
			fields[ev.Target] = make(interprocdomain.FieldValues)
		}
		fields[ev.Target][field] = mergeCapturedFieldValue(fields[ev.Target][field], fieldType)
	}
	if len(fields) == 0 {
		return nil
	}
	return fields
}

// FieldFactsFromAssignmentsAtPoint reduces lowered captured field assignments
// visible at observedPoint into captured field facts. Root rebindings are
// excluded; only paths below a captured root are admitted as mutable
// capture-surface evidence.
func FieldFactsFromAssignmentsAtPoint(inputs *flow.Inputs, capturedSyms functionsymbols.Set, observedPoint cfg.Point) map[cfg.SymbolID]interprocdomain.FieldValues {
	if inputs == nil || len(inputs.Assignments) == 0 || capturedSyms.IsEmpty() {
		return nil
	}
	facts := make(map[cfg.SymbolID]interprocdomain.FieldValues)
	for _, assignment := range inputs.Assignments {
		if observedPoint != 0 && assignment.Point > observedPoint {
			continue
		}
		if assignment.TargetPath.Symbol == 0 || len(assignment.TargetPath.Segments) == 0 || !capturedSyms.Contains(assignment.TargetPath.Symbol) {
			continue
		}
		ev := api.CapturedFieldEvidence{
			Point:      assignment.Point,
			Target:     assignment.TargetPath.Symbol,
			Field:      assignment.TargetPath.Segments[0].Name,
			TargetPath: assignment.TargetPath,
			ValueType:  assignment.Type,
		}
		field, fieldType, ok := capturedFieldFactValue(ev, assignment.Type)
		if !ok {
			continue
		}
		if facts[ev.Target] == nil {
			facts[ev.Target] = make(interprocdomain.FieldValues)
		}
		facts[ev.Target][field] = mergeCapturedFieldValue(facts[ev.Target][field], fieldType)
	}
	if len(facts) == 0 {
		return nil
	}
	return facts
}

// PromotedFieldsAtPoint reports, per captured symbol, the top-level field slots
// whose assignment definitely reaches observedPoint with a concrete, non-nilable
// value. Promotion is restricted to single-segment field writes whose assignment
// point dominates observedPoint: only those prove the field is present (no
// longer optional) at the capture boundary. Multi-segment writes (s.f.g = v) do
// not establish that s.f itself is present and are excluded.
func PromotedFieldsAtPoint(
	inputs *flow.Inputs,
	capturedSyms functionsymbols.Set,
	observedPoint cfg.Point,
	dominates func(assignPoint, observedPoint cfg.Point) bool,
) PromotedFields {
	if inputs == nil || len(inputs.Assignments) == 0 || capturedSyms.IsEmpty() || dominates == nil {
		return nil
	}
	promoted := make(PromotedFields)
	for _, assignment := range inputs.Assignments {
		path := assignment.TargetPath
		if path.Symbol == 0 || len(path.Segments) != 1 || !capturedSyms.Contains(path.Symbol) {
			continue
		}
		fieldKey, ok := capturedFieldSegmentKey(path.Segments[0])
		if !ok {
			continue
		}
		if assignment.Type == nil || unwrap.IsOptionalLike(assignment.Type) {
			continue
		}
		if !dominates(assignment.Point, observedPoint) {
			continue
		}
		if promoted[path.Symbol] == nil {
			promoted[path.Symbol] = make(PromotedFieldSet)
		}
		promoted[path.Symbol][fieldKey] = true
	}
	if len(promoted) == 0 {
		return nil
	}
	return promoted
}

func capturedFieldFactValue(ev api.CapturedFieldEvidence, leaf typ.Type) (interprocdomain.FieldKey, typ.Type, bool) {
	var field interprocdomain.FieldKey
	if ev.Field != "" {
		var ok bool
		field, ok = interprocdomain.FieldKeyFromName(ev.Field)
		if !ok {
			return interprocdomain.FieldKey{}, nil, false
		}
	}
	segments := ev.TargetPath.Segments
	if len(segments) > 0 {
		first, ok := capturedFieldSegmentKey(segments[0])
		if !ok {
			return interprocdomain.FieldKey{}, nil, false
		}
		field = first
		segments = segments[1:]
	}
	if _, ok := interprocdomain.FieldKeyStringKey(field); !ok {
		return interprocdomain.FieldKey{}, nil, false
	}
	return field, nestCapturedFieldValue(segments, leaf), true
}

func nestCapturedFieldValue(segments []constraint.Segment, leaf typ.Type) typ.Type {
	out := leaf
	if out == nil {
		out = typ.Unknown
	}
	for i := len(segments) - 1; i >= 0; i-- {
		name, ok := capturedFieldSegmentName(segments[i])
		if !ok {
			return out
		}
		out = typ.NewRecord().Field(name, out).Build()
	}
	return out
}

func capturedFieldSegmentName(seg constraint.Segment) (string, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		return seg.Name, seg.Name != ""
	default:
		return "", false
	}
}

func capturedFieldSegmentKey(seg constraint.Segment) (interprocdomain.FieldKey, bool) {
	switch seg.Kind {
	case constraint.SegmentField, constraint.SegmentIndexString:
		if seg.Name == "" {
			return interprocdomain.FieldKey{}, false
		}
		return seg, true
	default:
		return interprocdomain.FieldKey{}, false
	}
}

func mergeCapturedFieldValue(existing product.AbstractValue, next typ.Type) product.AbstractValue {
	if existing.IsZero() {
		return liftCarrierType(next)
	}
	return liftCarrierType(valueJoinPrecise(existing.ProjectValue(), next))
}

func valueJoinPrecise(existing, next typ.Type) typ.Type {
	if existing == nil {
		return next
	}
	if next == nil {
		return existing
	}
	return value.JoinPrecise(existing, next)
}
