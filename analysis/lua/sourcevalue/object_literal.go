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
	builder := typetable.NewConstructorBuilder()
	var expectedType typ.Type
	if expectedValue, ok := lit.Expected(); ok {
		expectedType, _ = typevalue.TypeOf(reg, expectedValue)
	}
	expected, hasExpected := luatypeprojection.ExpectedObjectLiteralRecord(expectedType, func(name string) (typ.Type, bool) {
		return objectLiteralDotFieldType(reg, typeValues, lit, resolve, name)
	})
	seen := false
	seenUntrustedTop := false
	for _, entry := range lit.Entries() {
		segs := entry.Suffix().Segments
		path, ok := luatypeprojection.ConstructorPathFromSegments(segs)
		if !ok {
			continue
		}
		value, ok := resolve(entry.Source())
		if !ok {
			if hasExpected {
				if filled, ok := luatypeprojection.ExpectedRecordField(expected, segs); ok {
					if !builder.Add(path, filled) {
						return nil, false
					}
					seen = true
				}
			}
			continue
		}
		t, ok := ObjectLiteralEntryType(reg, typeValues, value)
		if !ok {
			if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) {
				seenUntrustedTop = true
				continue
			}
			if hasExpected {
				if filled, ok := luatypeprojection.ExpectedRecordField(expected, segs); ok {
					if !builder.Add(path, filled) {
						return nil, false
					}
					seen = true
				}
			}
			continue
		}
		if hasExpected {
			if adopted, ok := luatypeprojection.AdoptExpectedFieldType(expected, segs, t); ok {
				if !builder.AddSealed(path, adopted) {
					return nil, false
				}
				seen = true
				continue
			}
		}
		if !builder.Add(path, t) {
			return nil, false
		}
		seen = true
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

func objectLiteralDotFieldType(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	lit factflow.ObjectLiteral,
	resolve func(factflow.ValueSource) (product.Value, bool),
	name string,
) (typ.Type, bool) {
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
		return ObjectLiteralEntryType(reg, typeValues, value)
	}
	return nil, false
}

func ObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if t, ok := typevalue.TypeOf(reg, value); ok {
		if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) && (typ.IsAny(t) || typ.IsUnknown(t)) {
			return nil, false
		}
		origin := product.Get(reg, value, variantoriginpkg.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if narrowed, ok := typeValues.NarrowVariantByOrigin(t, origin.Family(), origin.Cases()); ok {
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
