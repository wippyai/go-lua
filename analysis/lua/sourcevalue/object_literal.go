package sourcevalue

import (
	"github.com/wippyai/go-lua/analysis/domain/path/segment"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/escape"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/evidence"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/identity"
	variantoriginpkg "github.com/wippyai/go-lua/analysis/domain/value/axis/variantorigin"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
	luatypeprojection "github.com/wippyai/go-lua/analysis/lua/typeprojection"
	typetable "github.com/wippyai/go-lua/analysis/type/table"
	"github.com/wippyai/go-lua/analysis/type/typ"
)

func ObjectLiteralType(reg *axis.Registry, lit factflow.ObjectLiteral, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	return ObjectLiteralTypeCached(reg, nil, lit, resolver)
}

func ObjectLiteralTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteral, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	return ObjectLiteralTypeViewCached(reg, typeValues, lit.View(), resolver)
}

func ObjectLiteralTypeViewCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (typ.Type, bool) {
	builder := typetable.NewConstructorBuilder()
	entries := objectLiteralEntryQueries{
		reg:        reg,
		typeValues: typeValues,
		proofs:     proof.New(reg, typeValues),
		lit:        lit,
		resolver:   resolver,
	}
	var expectedType typ.Type
	if expectedValue, ok := lit.Expected(); ok {
		expectedType, _ = entries.proofs.ValueType(expectedValue)
	}
	expected, hasExpected := luatypeprojection.ExpectedObjectLiteralRecordCached(typeValues, expectedType, func(name string) (typ.Type, bool) {
		return entries.dotFieldType(name)
	})
	seen := false
	valid := true
	lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
		path, ok := constructorPathFromEntry(entry)
		if !ok {
			return true
		}
		value, ok := entries.value(entry)
		if !ok {
			if hasExpected {
				if filled, ok := expectedRecordEntryField(expected, entry); ok {
					if !builder.Add(path, filled) {
						valid = false
						return false
					}
					seen = true
					return true
				}
			}
			if expected, hasExpected := entry.Expected(); hasExpected {
				if t, expectedOK := entries.proofs.ValueType(expected); expectedOK {
					if !builder.Add(path, t) {
						valid = false
						return false
					}
					seen = true
					return true
				}
			}
			if !builder.Add(path, typ.Unknown) {
				valid = false
				return false
			}
			seen = true
			return true
		}
		t, ok := entries.entryType(entry)
		if !ok {
			if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) {
				if !builder.Add(path, untrustedObjectLiteralEntryType(reg, typeValues, value)) {
					valid = false
					return false
				}
				seen = true
				return true
			}
			if hasExpected {
				if filled, ok := expectedRecordEntryField(expected, entry); ok {
					if !builder.Add(path, filled) {
						valid = false
						return false
					}
					seen = true
					return true
				}
			}
			if expected, hasExpected := entry.Expected(); hasExpected {
				if t, expectedOK := entries.proofs.ValueType(expected); expectedOK {
					if !builder.Add(path, t) {
						valid = false
						return false
					}
					seen = true
					return true
				}
			}
			if !builder.Add(path, typ.Unknown) {
				valid = false
				return false
			}
			seen = true
			return true
		}
		if hasExpected {
			if adopted, ok := adoptExpectedEntryFieldType(typeValues, expected, entry, t); ok {
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
		empty := typetable.NewRecord().Build()
		if expectedType != nil && typeValues.IsFreshAssignable(empty, expectedType) {
			return expectedType, true
		}
		return empty, true
	}
	return builder.Build()
}

// ObjectLiteralValueFromViewCached evaluates a lowered object literal to the
// same product value used by the checker: a type witness for the constructed
// record, optional literal identity, and fresh escape evidence.
func ObjectLiteralValueFromViewCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, resolver factflow.ValueSourceResolver) (product.Value, bool) {
	t, ok := ObjectLiteralTypeViewCached(reg, typeValues, lit, resolver)
	if !ok {
		return product.Value{}, false
	}
	return ObjectLiteralValueFromTypeCached(reg, typeValues, lit, t), true
}

// ObjectLiteralValueFromTypeCached assembles the product value for an object
// literal after its record type has already been computed.
func ObjectLiteralValueFromTypeCached(reg *axis.Registry, typeValues *typevalue.Cache, lit factflow.ObjectLiteralView, t typ.Type) product.Value {
	value := typeValues.FromTypeWithWitness(reg, t)
	ed := product.Edit(reg, value)
	if id, ok := lit.Identity(); ok {
		product.EditSet(&ed, identity.Key, identity.Singleton(id))
	}
	product.EditSet(&ed, escape.Key, escape.Fresh())
	return ed.Done()
}

type objectLiteralEntryQueries struct {
	reg        *axis.Registry
	typeValues *typevalue.Cache
	proofs     proof.Reader
	lit        factflow.ObjectLiteralView
	resolver   factflow.ValueSourceResolver

	valuesSmall [objectLiteralEntryCacheInline]resolvedObjectLiteralValueEntry
	valuesLen   int
	values      map[factflow.ValueSource]resolvedObjectLiteralValue
	typesSmall  [objectLiteralEntryCacheInline]resolvedObjectLiteralTypeEntry
	typesLen    int
	types       map[factflow.ValueSource]resolvedObjectLiteralType
}

const objectLiteralEntryCacheInline = 4

type resolvedObjectLiteralValueEntry struct {
	source factflow.ValueSource
	value  product.Value
	ok     bool
}

type resolvedObjectLiteralValue struct {
	value product.Value
	ok    bool
}

type resolvedObjectLiteralTypeEntry struct {
	source factflow.ValueSource
	t      typ.Type
	ok     bool
}

type resolvedObjectLiteralType struct {
	t  typ.Type
	ok bool
}

func (q *objectLiteralEntryQueries) value(entry factflow.ObjectEntryView) (product.Value, bool) {
	source := entry.Source()
	if cached, ok := q.cachedValue(source); ok {
		return cached.value, cached.ok
	}
	if q.resolver == nil {
		q.rememberValue(source, product.Value{}, false)
		return product.Value{}, false
	}
	value, ok := q.resolver.ResolveValueSource(source)
	q.rememberValue(source, value, ok)
	return value, ok
}

func (q *objectLiteralEntryQueries) rememberValue(source factflow.ValueSource, value product.Value, ok bool) {
	if q.values == nil && q.valuesLen < len(q.valuesSmall) {
		q.valuesSmall[q.valuesLen] = resolvedObjectLiteralValueEntry{source: source, value: value, ok: ok}
		q.valuesLen++
		return
	}
	if q.values == nil {
		q.values = make(map[factflow.ValueSource]resolvedObjectLiteralValue, q.lit.EntryCount())
		for i := 0; i < q.valuesLen; i++ {
			entry := q.valuesSmall[i]
			q.values[entry.source] = resolvedObjectLiteralValue{value: entry.value, ok: entry.ok}
			q.valuesSmall[i] = resolvedObjectLiteralValueEntry{}
		}
	}
	q.values[source] = resolvedObjectLiteralValue{value: value, ok: ok}
}

func (q *objectLiteralEntryQueries) cachedValue(source factflow.ValueSource) (resolvedObjectLiteralValue, bool) {
	for i := 0; i < q.valuesLen; i++ {
		entry := q.valuesSmall[i]
		if entry.source == source {
			return resolvedObjectLiteralValue{value: entry.value, ok: entry.ok}, true
		}
	}
	if q.values == nil {
		return resolvedObjectLiteralValue{}, false
	}
	value, ok := q.values[source]
	return value, ok
}

func (q *objectLiteralEntryQueries) entryType(entry factflow.ObjectEntryView) (typ.Type, bool) {
	source := entry.Source()
	if cached, ok := q.cachedType(source); ok {
		return cached.t, cached.ok
	}
	value, ok := q.value(entry)
	if !ok {
		q.rememberType(source, nil, false)
		return nil, false
	}
	t, ok := ObjectLiteralEntryType(q.reg, q.typeValues, value)
	if !ok {
		if expected, hasExpected := entry.Expected(); hasExpected {
			t, ok = q.proofs.ValueType(expected)
		}
	}
	q.rememberType(source, t, ok)
	return t, ok
}

func (q *objectLiteralEntryQueries) rememberType(source factflow.ValueSource, t typ.Type, ok bool) {
	if q.types == nil && q.typesLen < len(q.typesSmall) {
		q.typesSmall[q.typesLen] = resolvedObjectLiteralTypeEntry{source: source, t: t, ok: ok}
		q.typesLen++
		return
	}
	if q.types == nil {
		q.types = make(map[factflow.ValueSource]resolvedObjectLiteralType, q.lit.EntryCount())
		for i := 0; i < q.typesLen; i++ {
			entry := q.typesSmall[i]
			q.types[entry.source] = resolvedObjectLiteralType{t: entry.t, ok: entry.ok}
			q.typesSmall[i] = resolvedObjectLiteralTypeEntry{}
		}
	}
	q.types[source] = resolvedObjectLiteralType{t: t, ok: ok}
}

func (q *objectLiteralEntryQueries) cachedType(source factflow.ValueSource) (resolvedObjectLiteralType, bool) {
	for i := 0; i < q.typesLen; i++ {
		entry := q.typesSmall[i]
		if entry.source == source {
			return resolvedObjectLiteralType{t: entry.t, ok: entry.ok}, true
		}
	}
	if q.types == nil {
		return resolvedObjectLiteralType{}, false
	}
	value, ok := q.types[source]
	return value, ok
}

func (q *objectLiteralEntryQueries) dotFieldType(name string) (typ.Type, bool) {
	var out typ.Type
	found := false
	q.lit.ForEachEntry(func(entry factflow.ObjectEntryView) bool {
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
		out, found = q.entryType(entry)
		return false
	})
	return out, found
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

func adoptExpectedEntryFieldType(typeValues *typevalue.Cache, rec *typ.Record, entry factflow.ObjectEntryView, inferred typ.Type) (typ.Type, bool) {
	if entry.SuffixSegmentCount() != 1 {
		return nil, false
	}
	seg, ok := entry.SuffixSegmentAt(0)
	if !ok {
		return nil, false
	}
	return luatypeprojection.AdoptExpectedSegmentTypeCached(typeValues, rec, seg, inferred)
}

func untrustedObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) typ.Type {
	if t, ok := typeValues.TypeOf(reg, value); ok && t != nil && (typ.IsAny(t) || typ.IsUnknown(t)) {
		return t
	}
	return typ.Unknown
}

func ObjectLiteralEntryType(reg *axis.Registry, typeValues *typevalue.Cache, value product.Value) (typ.Type, bool) {
	if t, ok := proof.New(reg, typeValues).ValueType(value); ok {
		if ObjectLiteralEntryHasUntrustedTopOrigin(reg, value) && (typ.IsAny(t) || typ.IsUnknown(t)) {
			return nil, false
		}
		origin := product.Get(reg, value, variantoriginpkg.Key)
		if !origin.IsBottom() && !origin.IsTop() {
			if narrowed, ok := typeValues.NarrowVariantByOriginView(t, origin.Family(), origin.CasesView()); ok {
				return narrowed, true
			}
		}
		return t, true
	}
	if t, ok := runtimeKindValueType(reg, value); ok {
		return t, true
	}
	return nil, false
}

func ObjectLiteralEntryHasUntrustedTopOrigin(reg *axis.Registry, value product.Value) bool {
	ev := product.Get(reg, value, evidence.Key)
	return ev.IsGradualTop() || ev.IsExplicitTop()
}

func ExpectedEntryAdmissibleCached(reg *axis.Registry, typeValues *typevalue.Cache, value, expected product.Value) bool {
	expectedType, hasExpected := typeValues.TypeOf(reg, expected)
	if !hasExpected || expectedType == nil {
		return true
	}
	actualType, hasActual := typeValues.TypeOf(reg, value)
	if !hasActual || actualType == nil {
		return true
	}
	return typeValues.IsFreshAssignable(actualType, expectedType)
}
