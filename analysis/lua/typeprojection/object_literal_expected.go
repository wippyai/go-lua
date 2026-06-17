package typeprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// ExpectedObjectLiteralRecord resolves the contextual record contract for a Lua
// object literal. Direct record contracts are used as-is; discriminated record
// unions select the unique arm matched by the literal's own field evidence.
func ExpectedObjectLiteralRecord(expected typ.Type, fieldType func(name string) (typ.Type, bool)) (*typ.Record, bool) {
	if rec, ok := reachRecord(expected, 0); ok {
		return rec, true
	}
	return selectObjectLiteralUnionArm(expected, fieldType, 0)
}

// ExpectedRecordField returns the declared type of a single-segment member on a
// contextual record type. Dot fields match record fields; bracket-string entries
// match exact static string members.
func ExpectedRecordField(rec *typ.Record, segs []segment.Segment) (typ.Type, bool) {
	if rec == nil || len(segs) != 1 {
		return nil, false
	}
	seg := segs[0]
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

// AdoptExpectedFieldType widens an inferred literal-entry type to the declared
// contextual member type when the inferred value is admissible to it.
func AdoptExpectedFieldType(rec *typ.Record, segs []segment.Segment, inferred typ.Type) (typ.Type, bool) {
	declared, ok := ExpectedRecordField(rec, segs)
	if !ok || declared == nil || inferred == nil {
		return nil, false
	}
	if typ.IsAny(declared) || typ.IsUnknown(declared) || typ.IsAny(inferred) || typ.IsUnknown(inferred) {
		return nil, false
	}
	if !subtype.IsSubtype(inferred, declared) {
		if subtype.IsFreshAssignable(inferred, declared) {
			return declared, true
		}
		return nil, false
	}
	return declared, true
}

func selectObjectLiteralUnionArm(expected typ.Type, fieldType func(name string) (typ.Type, bool), depth int) (*typ.Record, bool) {
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
		if !objectLiteralMatchesDiscriminants(rec, fieldType) {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = rec
	}
	return match, match != nil
}

func objectLiteralMatchesDiscriminants(rec *typ.Record, fieldType func(name string) (typ.Type, bool)) bool {
	discriminants := 0
	for _, field := range rec.Fields {
		if _, ok := unwrap.Annotated(field.Type).(*typ.Literal); !ok {
			continue
		}
		discriminants++
		got, ok := fieldType(field.Name)
		if !ok || !subtype.IsSubtype(got, field.Type) {
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
