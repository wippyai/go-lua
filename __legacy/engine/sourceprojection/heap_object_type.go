package sourceprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/domain/type/inspect"
	"github.com/wippyai/go-lua/analysis/domain/type/kind"
	"github.com/wippyai/go-lua/analysis/domain/type/normalize"
	"github.com/wippyai/go-lua/analysis/domain/type/subtype"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/domain/type/unwrap"
)

// HeapObjectContainerType projects the finite container shape owned by value's
// exact heap-table identity. It is intentionally stricter than a path read:
// callers may use the result as a type witness for value only when the heap
// object's root still matches the same exact identity.
func HeapObjectContainerType(reg *axis.Registry, typeValues *typevalue.Cache, in state.State, value product.Value) (typ.Type, bool) {
	if reg == nil {
		return nil, false
	}
	id, ok := identityvalue.ExactID(reg, value)
	if !ok {
		return nil, false
	}
	object := in.ReadHeapTableObject(reg, id)
	root := object.Root()
	rootID, ok := identityvalue.ExactID(reg, root)
	if !ok || rootID != id {
		return nil, false
	}
	return HeapObjectContainerTypeFromFactors(reg, typeValues, value, root, func(visit func(dynamicindex.Fact)) {
		object.VisitDynamicIndexFacts(func(_ dynamicindex.Key, fact dynamicindex.Fact) bool {
			visit(fact)
			return true
		})
	})
}

// HeapObjectContainerTypeFromFactors is the coordinate-native form of
// HeapObjectContainerType. It observes only the returned value, the matching
// object-root factor, and the skeleton-owned dynamic-index facts; static
// members are mathematically irrelevant to this projection and never need to
// be aligned into a whole heap row.
func HeapObjectContainerTypeFromFactors(
	reg *axis.Registry,
	typeValues *typevalue.Cache,
	value product.Value,
	root product.Value,
	visitFacts func(func(dynamicindex.Fact)),
) (typ.Type, bool) {
	if reg == nil || visitFacts == nil || !product.BelongsToRegistry(reg, value) || !product.BelongsToRegistry(reg, root) {
		return nil, false
	}
	// A gradual Any/Unknown witness is deliberate caller authority. Heap
	// evidence may refine precise witnesses, but must not replace that gradual
	// contract with a finite container spelling. This gate belongs at the
	// canonical factored projection seam so concrete and guarded execution
	// cannot drift.
	if typeValues != nil {
		if valueType, known := typeValues.TypeOf(reg, value); known && valueType != nil &&
			(typ.ContainsAny(valueType) || inspect.ContainsUnknown(valueType)) {
			return nil, false
		}
	}
	id, valueOK := identityvalue.ExactID(reg, value)
	rootID, rootOK := identityvalue.ExactID(reg, root)
	if !valueOK || !rootOK || rootID != id || product.Equal(reg, product.Meet(reg, root, value), product.Bottom(reg)) {
		return nil, false
	}
	return dynamicIndexContainerType(reg, typeValues, visitFacts)
}

func dynamicIndexContainerType(reg *axis.Registry, typeValues *typevalue.Cache, visitFacts func(func(dynamicindex.Fact))) (typ.Type, bool) {
	var keyTypes []typ.Type
	var valueTypes []typ.Type
	visitFacts(func(fact dynamicindex.Fact) {
		if fact.Admission != dynamicindex.AdmissionAdmitted {
			return
		}
		keyType, keyOK := proof.New(reg, typeValues).ValueTypeWithPresence(fact.KeyValue)
		valueType, valueOK := proof.New(reg, typeValues).ValueTypeWithPresence(fact.Value)
		if !keyOK || keyType == nil || !valueOK || valueType == nil {
			return
		}
		valueType = withoutEmptyRecordWitness(valueType)
		if valueType == nil {
			return
		}
		keyTypes = append(keyTypes, keyType)
		valueTypes = append(valueTypes, valueType)
	})
	if len(keyTypes) == 0 || len(valueTypes) == 0 {
		return nil, false
	}
	keyType := normalize.UnionForEvidence(keyTypes...)
	valueType := normalize.UnionForEvidence(valueTypes...)
	if subtype.IsSubtype(keyType, typ.Integer) {
		return typ.NewArray(valueType), true
	}
	return typ.NewMap(keyType, valueType), true
}

func withoutEmptyRecordWitness(t typ.Type) typ.Type {
	t = unwrap.Annotated(t)
	if emptyRecordWitness(t) {
		return nil
	}
	if union, ok := t.(*typ.Union); ok {
		members := make([]typ.Type, 0, len(union.Members))
		for _, member := range union.Members {
			if member == nil || member.Kind() == kind.Nil || emptyRecordWitness(member) {
				continue
			}
			members = append(members, member)
		}
		if len(members) == 0 {
			return nil
		}
		return normalize.UnionForEvidence(members...)
	}
	return t
}

func emptyRecordWitness(t typ.Type) bool {
	record, ok := unwrap.Annotated(t).(*typ.Record)
	return ok && record != nil &&
		len(record.Fields) == 0 &&
		len(record.StaticMembers) == 0 &&
		record.MapKey == nil &&
		record.MapValue == nil &&
		record.Metatable == nil &&
		!record.Open
}
