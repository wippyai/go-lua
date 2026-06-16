package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// Apply applies a declared type projection using Lua callable semantics.
func Apply(source typ.Type, p projection.Projection) (typ.Type, bool) {
	current := source
	for _, step := range p.Steps {
		switch step.Kind {
		case projection.StepField:
			next, ok := access.Field(current, step.Field)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepCallableReturn:
			next, ok := typecall.CallableReturn(current)
			if !ok {
				return nil, false
			}
			current = next
		case projection.StepGenericArg:
			if step.Index < 0 {
				return nil, false
			}
			inst, ok := unwrap.Alias(current).(*typ.Instantiated)
			if !ok || inst == nil || step.Index >= len(inst.TypeArgs) || inst.TypeArgs[step.Index] == nil {
				return nil, false
			}
			current = inst.TypeArgs[step.Index]
		case projection.StepInstantiateGeneric:
			g, ok := unwrap.Alias(step.Type).(*typ.Generic)
			if !ok || g == nil || len(g.TypeParams) != 1 || current == nil {
				return nil, false
			}

			payload := current
			if meta, ok := unwrap.Alias(payload).(*typ.Meta); ok && meta != nil && meta.Of != nil {
				payload = meta.Of
			}
			if payload == nil {
				return nil, false
			}
			if constraint := g.TypeParams[0].Constraint; constraint != nil && !subtype.IsSubtype(payload, constraint) {
				return nil, false
			}
			current = typ.Instantiate(g, payload)
		default:
			return nil, false
		}
	}
	return current, current != nil
}

// ApplySegments applies a field/index path suffix using the same projection
// semantics as the local path walkers that project Lua table-like types.
func ApplySegments(source typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := source
	for _, seg := range segments {
		var ok bool
		switch seg.Kind {
		case segment.SegmentField, segment.SegmentIndexString:
			current, ok = access.Field(current, seg.Name)
		case segment.SegmentIndexInt:
			current, ok = access.RuntimeIndex(current, typ.LiteralInt(int64(seg.Index)))
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return current, current != nil
}

// SegmentKeyType returns the literal Lua key type for a static path segment.
func SegmentKeyType(seg segment.Segment) (typ.Type, bool) {
	switch seg.Kind {
	case segment.SegmentField, segment.SegmentIndexString:
		return typ.LiteralString(seg.Name), true
	case segment.SegmentIndexInt:
		return typ.LiteralInt(int64(seg.Index)), true
	default:
		return nil, false
	}
}
