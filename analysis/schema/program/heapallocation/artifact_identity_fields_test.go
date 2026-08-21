package heapallocation_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapallocation"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

type allocationIdentityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type allocationIdentityOperations []allocationIdentityOperation

func (operations *allocationIdentityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, allocationIdentityOperation{kind: 'i', id: id})
	return true
}

func (operations *allocationIdentityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, allocationIdentityOperation{kind: 'u', value: value})
	return true
}

func (operations *allocationIdentityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, allocationIdentityOperation{kind: 'b', value: encoded})
	return true
}

func allocationIdentityID(value byte) identity.ContentID { return identity.ContentID{value} }

func TestArtifactIdentityFieldsPreserveAllocationAndFieldOrder(t *testing.T) {
	allocationID, rootID := allocationIdentityID(1), allocationIdentityID(2)
	fieldID, fieldSpan, selectorSpan := allocationIdentityID(3), allocationIdentityID(4), allocationIdentityID(5)
	valuesSpan, valuesID := allocationIdentityID(6), allocationIdentityID(7)
	field, fieldOK := heapallocation.NewField(fieldID, heapallocation.FieldKindKey, fieldSpan, selectorSpan, valuesSpan, valuesID, 8, true, true, 9, true)
	allocation, allocationOK := heapallocation.NewAllocation(allocationID, heapallocation.RoleTable, heapallocation.FormFinalOpen, rootID, 0, 1)
	if !fieldOK || !allocationOK {
		t.Fatal("heap allocation rows")
	}
	catalog, catalogOK := programcatalog.CatalogID(allocationIdentityID(200))
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := (programpublication.Publication{HeapAllocations: []heapallocation.Allocation{allocation}, HeapFields: []heapallocation.Field{field}}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal publication")
	}
	var got allocationIdentityOperations
	if !heapallocation.WriteArtifactIdentityFields(frozen, &got) {
		t.Fatal("write allocation identity fields")
	}
	i := func(id identity.ContentID) allocationIdentityOperation {
		return allocationIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) allocationIdentityOperation {
		return allocationIdentityOperation{kind: 'u', value: value}
	}
	b := func(value bool) allocationIdentityOperation {
		if value {
			return allocationIdentityOperation{kind: 'b', value: 1}
		}
		return allocationIdentityOperation{kind: 'b'}
	}
	want := allocationIdentityOperations{
		u(1), i(allocationID), u(uint64(heapallocation.RoleTable)), u(uint64(heapallocation.FormFinalOpen)), i(rootID), u(1),
		i(fieldID), u(uint64(heapallocation.FieldKindKey)), i(fieldSpan), i(selectorSpan), i(valuesSpan), i(valuesID), u(8), b(true), b(true), u(9), b(true),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("allocation identity operations = %#v, want %#v", got, want)
	}
}

func TestArtifactIdentityFieldsBindFieldSensitivity(t *testing.T) {
	write := func(normalized uint64) allocationIdentityOperations {
		field, fieldOK := heapallocation.NewField(allocationIdentityID(3), heapallocation.FieldKindKey, allocationIdentityID(4), allocationIdentityID(5), allocationIdentityID(6), allocationIdentityID(7), 8, true, true, normalized, true)
		allocation, allocationOK := heapallocation.NewAllocation(allocationIdentityID(1), heapallocation.RoleTable, heapallocation.FormFinalOpen, allocationIdentityID(2), 0, 1)
		if !fieldOK || !allocationOK {
			t.Fatal("heap allocation rows")
		}
		catalog, catalogOK := programcatalog.CatalogID(allocationIdentityID(byte(200 + normalized)))
		if !catalogOK {
			t.Fatal("catalog")
		}
		frozen, sealed := (programpublication.Publication{HeapAllocations: []heapallocation.Allocation{allocation}, HeapFields: []heapallocation.Field{field}}).Seal(catalog, identity.StoreID(1))
		if !sealed {
			t.Fatal("seal publication")
		}
		var operations allocationIdentityOperations
		if !heapallocation.WriteArtifactIdentityFields(frozen, &operations) {
			t.Fatal("write allocation identity fields")
		}
		return operations
	}
	if reflect.DeepEqual(write(9), write(10)) {
		t.Fatal("normalized field key did not enter artifact identity fields")
	}
}
