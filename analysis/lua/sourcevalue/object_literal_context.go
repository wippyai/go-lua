package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	"github.com/wippyai/go-lua/analysis/type/subst"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
)

// expectedRecord decodes the contextual record type carried by the literal's
// declared-type witness, when one is present and reaches a record. A union
// expected type (discriminated record union) resolves to the single arm the
// literal's entries match by discriminant, so a tagged literal assigned to a
// union location adopts that arm's contract.
func expectedRecord(reg *axis.Registry, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) (*typ.Record, bool) {
	value, ok := lit.Expected()
	if !ok {
		return nil, false
	}
	t, ok := typevalue.TypeOf(reg, value)
	if !ok {
		return nil, false
	}
	if rec, ok := reachRecord(t, 0); ok {
		return rec, true
	}
	return selectUnionArm(reg, t, lit, resolve, 0)
}

// selectUnionArm picks the record arm of a union whose discriminant literal
// fields are all matched by the object literal's corresponding dot-field
// entries. Selection requires a unique matching arm: an ambiguous match leaves
// the literal to its inferred type rather than guessing.
func selectUnionArm(reg *axis.Registry, t typ.Type, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool), depth int) (*typ.Record, bool) {
	if t == nil || depth > typ.DefaultRecursionDepth {
		return nil, false
	}
	union, ok := unwrap.Annotated(t).(*typ.Union)
	if !ok {
		return nil, false
	}
	var match *typ.Record
	for _, member := range union.Members {
		rec, ok := reachRecord(member, depth+1)
		if !ok {
			continue
		}
		if !objectLiteralMatchesDiscriminants(reg, rec, lit, resolve) {
			continue
		}
		if match != nil {
			return nil, false
		}
		match = rec
	}
	return match, match != nil
}

// objectLiteralMatchesDiscriminants reports whether every literal-typed
// discriminant field of the arm (a field whose declared type is a string/bool/int
// literal) is present in the object literal with a matching literal value. Arms
// with no literal-typed field are not discriminants and never match here.
func objectLiteralMatchesDiscriminants(reg *axis.Registry, rec *typ.Record, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) bool {
	discriminants := 0
	for _, field := range rec.Fields {
		if _, ok := unwrap.Annotated(field.Type).(*typ.Literal); !ok {
			continue
		}
		discriminants++
		got, ok := objectLiteralFieldType(reg, field.Name, lit, resolve)
		if !ok || !subtype.IsSubtype(got, field.Type) {
			return false
		}
	}
	return discriminants > 0
}

// objectLiteralFieldType returns the inferred type of a single-segment dot field
// of the object literal, when present and typeable.
func objectLiteralFieldType(reg *axis.Registry, name string, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) (typ.Type, bool) {
	for _, entry := range lit.Entries() {
		segs := entry.Suffix().Segments
		if len(segs) != 1 {
			continue
		}
		seg := segs[0]
		if seg.Kind != segment.SegmentField || seg.Name != name {
			continue
		}
		value, ok := resolve(entry.Source())
		if !ok {
			return nil, false
		}
		return ObjectLiteralEntryType(reg, nil, value)
	}
	return nil, false
}

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
