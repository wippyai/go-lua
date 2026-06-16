package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// expectedRecordField returns the declared type of a single-segment member on
// the contextual record type, used to fill literal entries whose value is
// otherwise untypeable. Dot fields match record fields; bracket-string entries
// match exact static string members. Multi-segment suffixes and integer indexes
// are left to their inferred type.
func expectedRecordField(hasExpected bool, rec *typ.Record, segs []segment.Segment) (typ.Type, bool) {
	if !hasExpected || rec == nil || len(segs) != 1 {
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

// adoptExpectedFieldType widens a literal entry's inferred type to the declared
// member type of the contextual record when the inferred type is admissible to it.
// A fresh table literal stored at an annotated location takes that location's
// declared member types (bidirectional checking), so a literal field like `1` or
// "created" carries the declared `integer` or status-union contract rather than
// its narrow singleton type. Only single-segment dot fields and static string
// members whose inferred value is a subtype of the declared member are adopted;
// otherwise the inferred type is kept so genuine mismatches still surface at the
// assignment site.
func adoptExpectedFieldType(hasExpected bool, rec *typ.Record, segs []segment.Segment, inferred typ.Type) (typ.Type, bool) {
	declared, ok := expectedRecordField(hasExpected, rec, segs)
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
