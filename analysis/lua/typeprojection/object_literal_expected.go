package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ExpectedObjectLiteralRecord resolves the contextual record contract for a Lua
// object literal. Direct record contracts are used as-is; discriminated record
// unions select the unique arm matched by the literal's own field evidence.
func ExpectedObjectLiteralRecord(expected typ.Type, fieldType func(name string) (typ.Type, bool)) (*typ.Record, bool) {
	return ExpectedObjectLiteralRecordCached(nil, expected, fieldType)
}

// ExpectedObjectLiteralRecordCached resolves the contextual record contract
// through the caller-owned type query cache.
func ExpectedObjectLiteralRecordCached(typeValues *typevalue.Cache, expected typ.Type, fieldType func(name string) (typ.Type, bool)) (*typ.Record, bool) {
	if rec, ok := reachRecord(expected, 0); ok {
		return rec, true
	}
	return selectObjectLiteralUnionArm(typeValues, expected, fieldType, 0)
}

// ReachesRecord reports whether a type expands to a record contract through
// alias, recursive, or instantiated wrappers.
func ReachesRecord(t typ.Type) bool {
	_, ok := reachRecord(t, 0)
	return ok
}

// ReachesTableContract reports whether a type expands to a table-like
// constructor contract through wrappers that preserve construction context.
func ReachesTableContract(t typ.Type) bool {
	return reachesTableContract(t, 0)
}

// ExpectedRecordField returns the declared type of a single-segment member on a
// contextual record type. Dot fields match record fields; bracket-string entries
// match exact static string members.
func ExpectedRecordField(rec *typ.Record, segs []segment.Segment) (typ.Type, bool) {
	if rec == nil || len(segs) != 1 {
		return nil, false
	}
	return ExpectedRecordSegment(rec, segs[0])
}

// ExpectedRecordSegment returns the declared type of a direct member segment on
// a contextual record type. Dot fields and bracket-string members stay distinct.
func ExpectedRecordSegment(rec *typ.Record, seg segment.Segment) (typ.Type, bool) {
	if rec == nil {
		return nil, false
	}
	switch seg.Kind {
	case segment.SegmentField:
		field := rec.GetField(seg.Name)
		if field == nil || field.Type == nil {
			return nil, false
		}
		return field.Type, true
	case segment.SegmentIndexString:
		member := rec.GetStaticStringIndex(seg.Name)
		if member == nil || member.Type == nil {
			return nil, false
		}
		return member.Type, true
	default:
		return nil, false
	}
}

// ExpectedTypeAtSegments returns the contextual expected type at a nested
// object-literal member path. It is intentionally contract-shaped rather than a
// runtime read: only exact record fields/static members and unambiguous unions
// participate.
func ExpectedTypeAtSegments(t typ.Type, segs []segment.Segment) (typ.Type, bool) {
	if len(segs) == 0 {
		return t, t != nil
	}
	t = unwrap.Alias(t)
	switch got := t.(type) {
	case *typ.Record:
		return expectedRecordTypeAtSegments(got, segs)
	case *typ.Union:
		var out typ.Type
		for _, member := range got.Members {
			next, ok := ExpectedTypeAtSegments(member, segs)
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
	default:
		return nil, false
	}
}

// MissingRequiredRecordField returns the first required field from a closed
// contextual record contract that is absent from the literal.
func MissingRequiredRecordField(expected typ.Type, present func(name string) bool) (string, bool) {
	record, ok := closedExpectedRecord(expected)
	if !ok {
		return "", false
	}
	for _, field := range record.Fields {
		if field.Optional || unwrap.IsOptionalLike(field.Type) {
			continue
		}
		if present != nil && present(field.Name) {
			continue
		}
		return field.Name, true
	}
	return "", false
}

func closedExpectedRecord(t typ.Type) (*typ.Record, bool) {
	record, ok := unwrap.Alias(t).(*typ.Record)
	if !ok || record == nil || record.Open {
		return nil, false
	}
	return record, true
}

func expectedRecordTypeAtSegments(rec *typ.Record, segs []segment.Segment) (typ.Type, bool) {
	if rec == nil || len(segs) == 0 {
		return nil, false
	}
	var next typ.Type
	switch seg := segs[0]; seg.Kind {
	case segment.SegmentField:
		field := rec.GetField(seg.Name)
		if field == nil {
			member := rec.GetStaticStringIndex(seg.Name)
			if member == nil {
				return nil, false
			}
			next = member.Type
			if member.Optional {
				next = typ.MaterializeOptional(next)
			}
		} else {
			next = field.Type
			if field.Optional {
				next = typ.MaterializeOptional(next)
			}
		}
	case segment.SegmentIndexString:
		member := rec.GetStaticStringIndex(seg.Name)
		if member == nil {
			return nil, false
		}
		next = member.Type
		if member.Optional {
			next = typ.MaterializeOptional(next)
		}
	case segment.SegmentIndexInt:
		member := rec.GetStaticIntIndex(int64(seg.Index))
		if member == nil {
			return nil, false
		}
		next = member.Type
		if member.Optional {
			next = typ.MaterializeOptional(next)
		}
	default:
		return nil, false
	}
	return ExpectedTypeAtSegments(next, segs[1:])
}

// AdoptExpectedFieldType widens an inferred literal-entry type to the declared
// contextual member type when the inferred value is admissible to it.
func AdoptExpectedFieldType(rec *typ.Record, segs []segment.Segment, inferred typ.Type) (typ.Type, bool) {
	return AdoptExpectedFieldTypeCached(nil, rec, segs, inferred)
}

func AdoptExpectedFieldTypeCached(typeValues *typevalue.Cache, rec *typ.Record, segs []segment.Segment, inferred typ.Type) (typ.Type, bool) {
	if len(segs) != 1 {
		return nil, false
	}
	return AdoptExpectedSegmentTypeCached(typeValues, rec, segs[0], inferred)
}

// AdoptExpectedSegmentType widens an inferred direct member type to the
// declared contextual member type when the inferred value is admissible to it.
func AdoptExpectedSegmentType(rec *typ.Record, seg segment.Segment, inferred typ.Type) (typ.Type, bool) {
	return AdoptExpectedSegmentTypeCached(nil, rec, seg, inferred)
}

// AdoptExpectedSegmentTypeCached widens an inferred direct member type through
// the caller-owned type query cache.
func AdoptExpectedSegmentTypeCached(typeValues *typevalue.Cache, rec *typ.Record, seg segment.Segment, inferred typ.Type) (typ.Type, bool) {
	declared, ok := ExpectedRecordSegment(rec, seg)
	if !ok || declared == nil || inferred == nil {
		return nil, false
	}
	if typ.IsAny(inferred) || typ.IsUnknown(inferred) {
		return nil, false
	}
	if !typeValues.IsSubtype(inferred, declared) {
		if typeValues.IsFreshAssignable(inferred, declared) {
			return declared, true
		}
		return nil, false
	}
	return declared, true
}

func selectObjectLiteralUnionArm(typeValues *typevalue.Cache, expected typ.Type, fieldType func(name string) (typ.Type, bool), depth int) (*typ.Record, bool) {
	if expected == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	union, ok := unwrap.Annotated(expected).(*typ.Union)
	if !ok {
		return nil, false
	}
	var match *typ.Record
	for _, member := range union.Members {
		rec, ok := reachRecord(member, depth+1)
		if !ok {
			continue
		}
		if !objectLiteralMatchesDiscriminants(typeValues, rec, fieldType) {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = rec
	}
	return match, match != nil
}

func objectLiteralMatchesDiscriminants(typeValues *typevalue.Cache, rec *typ.Record, fieldType func(name string) (typ.Type, bool)) bool {
	discriminants := 0
	for _, field := range rec.Fields {
		if _, ok := unwrap.Annotated(field.Type).(*typ.Literal); !ok {
			continue
		}
		discriminants++
		got, ok := fieldType(field.Name)
		if !ok || !typeValues.IsSubtype(got, field.Type) {
			return false
		}
	}
	return discriminants > 0
}

func reachRecord(t typ.Type, depth int) (*typ.Record, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record:
		return v, true
	case *typ.Alias:
		return reachRecord(v.UnaliasedTarget(), depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return nil, false
		}
		return reachRecord(v.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return nil, false
		}
		return reachRecord(expanded, depth+1)
	default:
		return nil, false
	}
}

func reachesTableContract(t typ.Type, depth int) bool {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return false
	}
	switch v := unwrap.Annotated(t).(type) {
	case *typ.Record, *typ.Map, *typ.ReadonlyMap, *typ.Array, *typ.Tuple:
		return true
	case *typ.Alias:
		return reachesTableContract(v.UnaliasedTarget(), depth+1)
	case *typ.Optional:
		return reachesTableContract(v.Inner, depth+1)
	case *typ.Recursive:
		if v.Body == nil || v.Body == t {
			return false
		}
		return reachesTableContract(v.Body, depth+1)
	case *typ.Instantiated:
		expanded := subst.ExpandInstantiated(v)
		if expanded == nil || expanded == t {
			return false
		}
		return reachesTableContract(expanded, depth+1)
	case *typ.Union:
		for _, member := range v.Members {
			if reachesTableContract(member, depth+1) {
				return true
			}
		}
		return false
	default:
		return false
	}
}
