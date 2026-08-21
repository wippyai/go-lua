package heapindex_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
	"github.com/wippyai/go-lua/analysis/schema/program/heapindex"
	programpublication "github.com/wippyai/go-lua/analysis/schema/program/publication"
)

type indexIdentityOperation struct {
	kind  byte
	id    identity.ContentID
	value uint64
}

type indexIdentityOperations []indexIdentityOperation

func (operations *indexIdentityOperations) WriteContentID(id identity.ContentID) bool {
	*operations = append(*operations, indexIdentityOperation{kind: 'i', id: id})
	return true
}

func (operations *indexIdentityOperations) WriteUint(value uint64) bool {
	*operations = append(*operations, indexIdentityOperation{kind: 'u', value: value})
	return true
}

func (operations *indexIdentityOperations) WriteBool(value bool) bool {
	encoded := uint64(0)
	if value {
		encoded = 1
	}
	*operations = append(*operations, indexIdentityOperation{kind: 'b', value: encoded})
	return true
}

func indexIdentityID(value byte) identity.ContentID { return identity.ContentID{value} }

func TestArtifactIdentityFieldsPreserveIndexOrder(t *testing.T) {
	rowID, baseID, valuesSpan, valuesID := indexIdentityID(1), indexIdentityID(2), indexIdentityID(3), indexIdentityID(4)
	row, rowOK := heapindex.NewIndex(rowID, false, baseID, identity.ContentID{}, identity.ContentID{}, heapindex.LensExact, 7, valuesSpan, valuesID, 2)
	if !rowOK {
		t.Fatal("heap index row")
	}
	catalog, catalogOK := programcatalog.CatalogID(indexIdentityID(200))
	if !catalogOK {
		t.Fatal("catalog")
	}
	frozen, sealed := (programpublication.Publication{HeapIndexes: []heapindex.Index{row}}).Seal(catalog, identity.StoreID(1))
	if !sealed {
		t.Fatal("seal publication")
	}
	var got indexIdentityOperations
	if !heapindex.WriteArtifactIdentityFields(frozen, &got) {
		t.Fatal("write index identity fields")
	}
	i := func(id identity.ContentID) indexIdentityOperation {
		return indexIdentityOperation{kind: 'i', id: id}
	}
	u := func(value uint64) indexIdentityOperation {
		return indexIdentityOperation{kind: 'u', value: value}
	}
	b := func(value bool) indexIdentityOperation {
		if value {
			return indexIdentityOperation{kind: 'b', value: 1}
		}
		return indexIdentityOperation{kind: 'b'}
	}
	want := indexIdentityOperations{
		u(1), i(rowID), b(false), i(baseID), i(identity.ContentID{}), i(identity.ContentID{}), u(uint64(heapindex.LensExact)), u(7), i(valuesSpan), i(valuesID), u(3),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("index identity operations = %#v, want %#v", got, want)
	}
}

func TestArtifactIdentityFieldsBindIndexSensitivity(t *testing.T) {
	write := func(position int) indexIdentityOperations {
		row, rowOK := heapindex.NewIndex(indexIdentityID(1), false, indexIdentityID(2), identity.ContentID{}, identity.ContentID{}, heapindex.LensExact, 7, indexIdentityID(3), indexIdentityID(4), position)
		if !rowOK {
			t.Fatal("heap index row")
		}
		catalog, catalogOK := programcatalog.CatalogID(indexIdentityID(byte(200 + position)))
		if !catalogOK {
			t.Fatal("catalog")
		}
		frozen, sealed := (programpublication.Publication{HeapIndexes: []heapindex.Index{row}}).Seal(catalog, identity.StoreID(1))
		if !sealed {
			t.Fatal("seal publication")
		}
		var operations indexIdentityOperations
		if !heapindex.WriteArtifactIdentityFields(frozen, &operations) {
			t.Fatal("write index identity fields")
		}
		return operations
	}
	if reflect.DeepEqual(write(2), write(3)) {
		t.Fatal("index position did not enter artifact identity fields")
	}
}
