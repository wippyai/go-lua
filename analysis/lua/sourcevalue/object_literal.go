package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	variantoriginpkg "github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func ObjectLiteralType(reg *axis.Registry, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) (typ.Type, bool) {
	return ObjectLiteralTypeCached(reg, nil, lit, resolve)
}

func ObjectLiteralTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteral, resolve func(factflow.ValueSource) (product.Value, bool)) (typ.Type, bool) {
	return ObjectLiteralTypeViewCached(reg, typeValues, lit.View(), resolve)
}

func ObjectLiteralTypeViewCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, resolve func(factflow.ValueSource) (product.Value, bool)) (typ.Type, bool) {
	builder := typetable.NewConstructorBuilder()
	var expectedType typ.Type
	if expectedValue, ok := lit.Expected(); ok {
		expectedType, _ = typevalue.TypeOf(reg, expectedValue)
	}
	expected, hasExpected := luatypeprojection.ExpectedObjectLiteralRecord(expectedType, func(name string) (typ.Type, bool) {
		return objectLiteralDotFieldTypeView(reg, typeValues, lit, resolve, name)
	})
	seen := false
	seenUntrustedTop := false
	valid := true
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		path, ok := constructorPathFromEntry(entry)
		if !ok {
			return true
		}
		value, ok := resolve(entry.Source())
		if !ok {
			if hasExpected {
				if filled, ok := expectedRecordEntryField(expected, entry); ok {
					if !builder.Add(path, filled) {
						valid = false
						return false
					}
					seen = true
				}
			}
			return true
		}
		t, ok := ObjectLiteralEntryType(reg, typeValues, value)
		if !ok {
			if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) {
				seenUntrustedTop = true
				return true
			}
			if hasExpected {
				if filled, ok := expectedRecordEntryField(expected, entry); ok {
					if !builder.Add(path, filled) {
						valid = false
						return false
					}
					seen = true
				}
			}
			return true
		}
		if hasExpected {
			if adopted, ok := adoptExpectedEntryFieldType(expected, entry, t); ok {
				if !builder.AddSealed(path, adopted) {
					valid = false
					return false
				}
				seen = true
				return true
			}
		}
		if !builder.Add(path, t) {
			valid = false
			return false
		}
		seen = true
		return true
	})
	if !valid {
		return nil, false
	}
	if !seen {
		if seenUntrustedTop {
			return typetable.NewRecord().Build(), true
		}
		if expectedType != nil {
			return expectedType, true
		}
		return typetable.NewRecord().Build(), true
	}
	return builder.Build()
}

func constructorPathFromEntry(entry factflow.ObjectEntryView) ([]typetable.ConstructorKey, bool) {
	return luatypeprojection.ConstructorPathFromSegmentReader(entry.SuffixSegmentCount(), entry.SuffixSegmentAt)
}

func expectedRecordEntryField(rec *typ.Record, entry factflow.ObjectEntryView) (typ.Type, bool) {
	if entry.SuffixSegmentCount() != 1 {
		return nil, false
	}
	seg, ok := entry.SuffixSegmentAt(0)
	if !ok {
		return nil, false
	}
	return luatypeprojection.ExpectedRecordSegment(rec, seg)
}

func adoptExpectedEntryFieldType(rec *typ.Record, entry factflow.ObjectEntryView, inferred typ.Type) (typ.Type, bool) {
	if entry.SuffixSegmentCount() != 1 {
		return nil, false
	}
	seg, ok := entry.SuffixSegmentAt(0)
	if !ok {
		return nil, false
	}
	return luatypeprojection.AdoptExpectedSegmentType(rec, seg, inferred)
}

func objectLiteralDotFieldTypeView(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	lit factflow.ObjectLiteralView,
	resolve func(factflow.ValueSource) (product.Value, bool),
	name string,
) (typ.Type, bool) {
	var out typ.Type
	found := false
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		if entry.SuffixSegmentCount() != 1 {
			return true
		}
		seg, ok := entry.SuffixSegmentAt(0)
		if !ok {
			return true
		}
		if seg.Kind != segment.SegmentField || seg.Name != name {
			return true
		}
		value, ok := resolve(entry.Source())
		if !ok {
			return false
		}
		out, found = ObjectLiteralEntryType(reg, typeValues, value)
		return false
	})
	return out, found
}

func ObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if t, ok := typevalue.TypeOf(reg, value); ok {
		if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) && (typ.IsAny(t) || typ.IsUnknown(t)) {
			return nil, false
		}
		origin := product.Get(reg, value, variantoriginpkg.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if narrowed, ok := typeValues.NarrowVariantByOrigin(t, origin.Family(), origin.CasesRef()); ok {
				return narrowed, true
			}
		}
		return t, true
	}
	return nil, false
}

func ObjectLiteralEntryHasUntrustedTopOrigin(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}
