package captured

import (
	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/flow"
	"github.com/wippyai/go-lua/types/subtype"
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

// MutatorValueType observes a lowered mutator value after flow solving.
type MutatorValueType func(cfg.Point, constraint.Path, typ.Type, flow.ValueTemplate) typ.Type

// MutatorKeyType observes a lowered mutator key after flow solving.
type MutatorKeyType func(cfg.Point, constraint.Path, typ.Type) typ.Type

// MutatorTypeObservers holds solved abstract-state readers for captured
// mutator evidence. Evidence reducers consume these readers instead of
// re-synthesizing AST expressions after flow lowering.
type MutatorTypeObservers struct {
	Value MutatorValueType
	Key   MutatorKeyType
}

// FieldFactsFromEvidence reduces captured field assignment evidence into
// canonical captured-field facts.
func FieldFactsFromEvidence(evidence []api.CapturedFieldEvidence, observe AssignedValueType) map[cfg.SymbolID]map[string]typ.Type {
	if len(evidence) == 0 {
		return nil
	}
	fields := make(map[cfg.SymbolID]map[string]typ.Type)
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
			fields[ev.Target] = make(map[string]typ.Type)
		}
		if existing := fields[ev.Target][field]; existing != nil {
			fields[ev.Target][field] = value.JoinPrecise(existing, fieldType)
		} else {
			fields[ev.Target][field] = fieldType
		}
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
func FieldFactsFromAssignmentsAtPoint(inputs *flow.Inputs, capturedSyms map[cfg.SymbolID]bool, observedPoint cfg.Point) map[cfg.SymbolID]map[string]typ.Type {
	if inputs == nil || len(inputs.Assignments) == 0 || len(capturedSyms) == 0 {
		return nil
	}
	facts := make(map[cfg.SymbolID]map[string]typ.Type)
	for _, assignment := range inputs.Assignments {
		if observedPoint != 0 && assignment.Point > observedPoint {
			continue
		}
		if assignment.TargetPath.Symbol == 0 || len(assignment.TargetPath.Segments) == 0 || !capturedSyms[assignment.TargetPath.Symbol] {
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
			facts[ev.Target] = make(map[string]typ.Type)
		}
		if existing := facts[ev.Target][field]; existing != nil {
			facts[ev.Target][field] = value.JoinPrecise(existing, fieldType)
		} else {
			facts[ev.Target][field] = fieldType
		}
	}
	if len(facts) == 0 {
		return nil
	}
	return facts
}

// PromotedFieldNamesAtPoint reports, per captured symbol, the top-level field
// names whose assignment definitely reaches observedPoint with a concrete,
// non-nilable value. Promotion is restricted to single-segment field writes
// whose assignment point dominates observedPoint: only those prove the field is
// present (no longer optional) at the capture boundary. Multi-segment writes
// (s.f.g = v) do not establish that s.f itself is present and are excluded.
func PromotedFieldNamesAtPoint(
	inputs *flow.Inputs,
	capturedSyms map[cfg.SymbolID]bool,
	observedPoint cfg.Point,
	dominates func(assignPoint, observedPoint cfg.Point) bool,
) map[cfg.SymbolID]map[string]bool {
	if inputs == nil || len(inputs.Assignments) == 0 || len(capturedSyms) == 0 || dominates == nil {
		return nil
	}
	promoted := make(map[cfg.SymbolID]map[string]bool)
	for _, assignment := range inputs.Assignments {
		path := assignment.TargetPath
		if path.Symbol == 0 || len(path.Segments) != 1 || !capturedSyms[path.Symbol] {
			continue
		}
		name, ok := capturedFieldSegmentName(path.Segments[0])
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
			promoted[path.Symbol] = make(map[string]bool)
		}
		promoted[path.Symbol][name] = true
	}
	if len(promoted) == 0 {
		return nil
	}
	return promoted
}

func capturedFieldFactValue(ev api.CapturedFieldEvidence, leaf typ.Type) (string, typ.Type, bool) {
	field := ev.Field
	segments := ev.TargetPath.Segments
	if len(segments) > 0 {
		first, ok := capturedFieldSegmentName(segments[0])
		if !ok {
			return "", nil, false
		}
		field = first
		segments = segments[1:]
	}
	if field == "" {
		return "", nil, false
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

// ContainerMutationsFromEvidence reduces captured container mutation evidence
// into canonical captured-container mutation facts.
func ContainerMutationsFromEvidence(evidence []api.CapturedContainerEvidence, observe MutatorTypeObservers) map[cfg.SymbolID][]api.ContainerMutation {
	if len(evidence) == 0 {
		return nil
	}
	mutations := make(map[cfg.SymbolID][]api.ContainerMutation)
	for _, ev := range evidence {
		if ev.Target == 0 {
			continue
		}
		valueType := ev.ValueType
		if observe.Value != nil {
			if t := observe.Value(ev.Point, ev.ValuePath, ev.ValueType, ev.ValueTemplate); t != nil {
				valueType = t
			}
		}
		if valueType == nil {
			valueType = typ.Unknown
		}
		keyType := ev.KeyType
		if observe.Key != nil {
			if t := observe.Key(ev.Point, ev.KeyPath, ev.KeyType); t != nil {
				keyType = t
			}
		}
		if ev.Kind == api.ContainerMutationMapElement && keyType == nil {
			keyType = typ.Unknown
		}
		mutations[ev.Target] = append(mutations[ev.Target], api.ContainerMutation{
			Kind:      ev.Kind,
			Segments:  cloneSegments(ev.Segments),
			KeyType:   liftCarrierType(subtype.WidenForInference(keyType)),
			ValueMode: ev.ValueMode,
			ValueType: liftCarrierType(subtype.WidenForInference(valueType)),
		})
	}
	if len(mutations) == 0 {
		return nil
	}
	return mutations
}

func cloneSegments(segments []constraint.Segment) []constraint.Segment {
	if len(segments) == 0 {
		return nil
	}
	out := make([]constraint.Segment, len(segments))
	copy(out, segments)
	return out
}
