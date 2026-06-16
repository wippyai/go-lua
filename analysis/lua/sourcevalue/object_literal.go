package sourcevalue

import (
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
	expected, hasExpected := expectedRecord(reg, lit, resolve)
	seen := false
	for _, entry := range lit.Entries() {
		segs := entry.Suffix().Segments
		path, ok := luatypeprojection.ConstructorPathFromSegments(segs)
		if !ok {
			continue
		}
		value, ok := resolve(entry.Source())
		if !ok {
			if filled, ok := expectedRecordField(hasExpected, expected, segs); ok {
				if !builder.Add(path, filled) {
					return nil, false
				}
				seen = true
			}
			continue
		}
		t, ok := ObjectLiteralEntryType(reg, typeValues, value)
		if !ok {
			if filled, ok := expectedRecordField(hasExpected, expected, segs); ok {
				if !builder.Add(path, filled) {
					return nil, false
				}
				seen = true
			}
			continue
		}
		if adopted, ok := adoptExpectedFieldType(hasExpected, expected, segs, t); ok {
			if !builder.AddSealed(path, adopted) {
				return nil, false
			}
			seen = true
			continue
		}
		if !builder.Add(path, t) {
			return nil, false
		}
		seen = true
	}
	if !seen {
		return nil, false
	}
	return builder.Build()
}

func ObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if t, ok := typevalue.TypeOf(reg, value); ok {
		origin := product.Get(reg, value, variantoriginpkg.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if narrowed, ok := typeValues.NarrowVariantByOrigin(t, origin.Family(), origin.Cases()); ok {
				return narrowed, true
			}
		}
		return t, true
	}
	ev := product.Get(reg, value, evidence.Key)
	if ev.IsGradualTop() || ev.IsExplicitTop() {
		return typ.Any, true
	}
	return nil, false
}
