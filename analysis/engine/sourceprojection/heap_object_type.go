package sourceprojection

import (
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/identityvalue"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/proof"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/engine/dynamicindex"
	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/type/kind"
	"github.com/wippyai/go-lua/analysis/type/normalize"
	"github.com/wippyai/go-lua/analysis/type/subtype"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/unwrap"
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
	if !ok || rootID != id || product.Equal(reg, product.Meet(reg, root, value), product.Bottom(reg)) {
		return nil, false
	}
	return dynamicIndexContainerType(reg, typeValues, object.DynamicIndexFacts())
}

func dynamicIndexContainerType(reg *axis.Registry, typeValues *typevalue.Cache, facts map[dynamicindex.Key]dynamicindex.Fact) (typ.Type, bool) {
	if len(facts) == 0 {
		return nil, false
	}
	var keyTypes []typ.Type
	var valueTypes []typ.Type
	for _, fact := range facts {
		if fact.Admission != dynamicindex.AdmissionAdmitted {
			continue
		}
		keyType, keyOK := proof.New(reg, typeValues).ValueTypeWithPresence(fact.KeyValue)
		valueType, valueOK := proof.New(reg, typeValues).ValueTypeWithPresence(fact.Value)
		if !keyOK || keyType == nil || !valueOK || valueType == nil {
			continue
		}
		valueType = withoutEmptyRecordWitness(valueType)
		if valueType == nil {
			continue
		}
		keyTypes = append(keyTypes, keyType)
		valueTypes = append(valueTypes, valueType)
	}
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
