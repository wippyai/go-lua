package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/lua/typecall"
	"github.com/wippyai/go-lua/analysis/type/access"
	"github.com/wippyai/go-lua/analysis/type/projection"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
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

// ApplyWriteSegments applies a path suffix as an assignment target contract.
// Field access keeps normal declared-member semantics, while bracket-index
// access uses write semantics: the target contract does not gain nil merely
// because reading the same dynamic slot could miss.
func ApplyWriteSegments(source typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := source
	for _, seg := range segments {
		var ok bool
		switch seg.Kind {
		case segment.SegmentField:
			current, ok = access.Field(current, seg.Name)
		case segment.SegmentIndexString:
			current, ok = access.WritableIndex(current, typ.LiteralString(seg.Name))
		case segment.SegmentIndexInt:
			current, ok = access.WritableIndex(current, typ.LiteralInt(int64(seg.Index)))
		default:
			return nil, false
		}
		if !ok {
			return nil, false
		}
	}
	return current, current != nil
}

// ApplyConstructorSegments applies a constructor path to a declared type. Unlike
// ordinary reads, every non-leaf prefix in a table constructor is present by
// construction: `{ error = { type = "x" } }` proves `error` is present while
// projecting the nested `error.type` contract, even if the declared field is
// optional.
func ApplyConstructorSegments(source typ.Type, segments []segment.Segment) (typ.Type, bool) {
	current := source
	for i, seg := range segments {
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
		if i < len(segments)-1 {
			if present := typetable.PresentReadonlyEntryValue(current); present != nil {
				current = present
			}
		}
	}
	return current, current != nil
}

// PresentConstructorRoot returns the contract seen by fields inside a table
// constructor. A constructor proves its root entry exists, so projecting fields
// from an optional table contract starts from the present table member.
func PresentConstructorRoot(declared typ.Type) typ.Type {
	if root := typetable.PresentReadonlyEntryValue(declared); root != nil {
		return root
	}
	return declared
}

// ExpectedConstructorEntryType resolves the contextual contract for an explicit
// table-constructor entry. It removes absence introduced by dynamic table reads
// such as array/map slots, but preserves declared nilability on record members:
// `{ nest = nil }` is valid when the field type is `Node?`.
func ExpectedConstructorEntryType(source typ.Type, segments []segment.Segment) (typ.Type, bool) {
	return expectedConstructorEntryType(source, segments, 0)
}

func expectedConstructorEntryType(source typ.Type, segments []segment.Segment, depth int) (typ.Type, bool) {
	if depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	if len(segments) == 0 {
		return source, source != nil
	}
	current := PresentConstructorRoot(source)
	current = unwrap.Alias(current)
	seg := segments[0]
	leaf := len(segments) == 1

	switch t := current.(type) {
	case *typ.Record:
		next, ok := constructorRecordSegment(t, seg)
		if !ok {
			return nil, false
		}
		if !leaf {
			next = PresentConstructorRoot(next)
		}
		return expectedConstructorEntryType(next, segments[1:], depth+1)
	case *typ.Array:
		if seg.Kind != segment.SegmentIndexInt {
			return nil, false
		}
		next := t.Element
		if next == nil {
			next = typ.Unknown
		}
		if !leaf {
			next = PresentConstructorRoot(next)
		}
		return expectedConstructorEntryType(next, segments[1:], depth+1)
	case *typ.Map:
		next, ok := constructorMapSegment(t.Key, t.Value, seg)
		if !ok {
			return nil, false
		}
		if !leaf {
			next = PresentConstructorRoot(next)
		}
		return expectedConstructorEntryType(next, segments[1:], depth+1)
	case *typ.ReadonlyMap:
		next, ok := constructorMapSegment(t.Key, t.Value, seg)
		if !ok {
			return nil, false
		}
		if !leaf {
			next = PresentConstructorRoot(next)
		}
		return expectedConstructorEntryType(next, segments[1:], depth+1)
	case *typ.Tuple:
		if seg.Kind != segment.SegmentIndexInt || seg.Index <= 0 || seg.Index > len(t.Elements) {
			return nil, false
		}
		next := t.Elements[seg.Index-1]
		if next == nil {
			next = typ.Unknown
		}
		if !leaf {
			next = PresentConstructorRoot(next)
		}
		return expectedConstructorEntryType(next, segments[1:], depth+1)
	case *typ.Union:
		return constructorUnionEntryType(t, segments, depth+1)
	default:
		return nil, false
	}
}

func constructorRecordSegment(rec *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if rec == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		if field := rec.GetField(seg.Name); field != nil {
			return constructorRecordEntryType(field.Type, field.Optional), true
		}
		if member := rec.GetStaticStringIndex(seg.Name); member != nil {
			return constructorRecordEntryType(member.Type, member.Optional), true
		}
	case segment.SegmentIndexString:
		if member := rec.GetStaticStringIndex(seg.Name); member != nil {
			return constructorRecordEntryType(member.Type, member.Optional), true
		}
	case segment.SegmentIndexInt:
		if member := rec.GetStaticIntIndex(int64(seg.Index)); member != nil {
			return constructorRecordEntryType(member.Type, member.Optional), true
		}
	}
	return nil, false
}

func constructorRecordEntryType(t typ.Type, optional bool) typ.Type {
	if t == nil {
		t = typ.Unknown
	}
	if optional {
		t = typ.MaterializeOptional(t)
	}
	return t
}

func constructorMapSegment(keyDomain typ.Type, value typ.Type, seg segment.Segment) (typ.Type, bool) {
	key, ok := SegmentKeyType(seg)
	if !ok || !typetable.MapComponentKeyAdmitsType(keyDomain, key) {
		return nil, false
	}
	if value == nil {
		value = typ.Unknown
	}
	return value, true
}

func constructorUnionEntryType(u *typ.Union, segments []segment.Segment, depth int) (typ.Type, bool) {
	if u == nil || len(u.Members) == 0 {
		return nil, false
	}
	var out typ.Type
	for _, member := range u.Members {
		next, ok := expectedConstructorEntryType(member, segments, depth+1)
		if !ok || next == nil {
			return nil, false
		}
		if out == nil {
			out = next
			continue
		}
		if !typ.TypeEquals(out, next) {
			return nil, false
		}
	}
	return out, out != nil
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
