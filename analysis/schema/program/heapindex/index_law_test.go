package heapindex

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	programcatalog "github.com/wippyai/go-lua/analysis/schema/program/catalog"
)

func indexLawID(t *testing.T, label string) identity.ContentID {
	t.Helper()
	id, ok := identity.DeriveContentID("analysis/schema/program/heap-index-law", []byte(label))
	if !ok {
		t.Fatalf("derive %s", label)
	}
	return id
}

func TestIndexAdmitsExactAndDynamicReadWriteShapes(t *testing.T) {
	id, base, result := indexLawID(t, "id"), indexLawID(t, "base"), indexLawID(t, "result")
	key, values, valuesID := indexLawID(t, "key"), indexLawID(t, "values"), indexLawID(t, "values-id")
	rows := []struct {
		name string
		row  Index
	}{
		{"exact read", mustIndex(t, id, true, base, result, identity.ContentID{}, LensExact, 7, identity.ContentID{}, identity.ContentID{}, -1)},
		{"dynamic read", mustIndex(t, id, true, base, result, key, LensDynamic, 0, identity.ContentID{}, identity.ContentID{}, -1)},
		{"exact write", mustIndex(t, id, false, base, identity.ContentID{}, identity.ContentID{}, LensExact, 7, values, valuesID, 2)},
		{"dynamic write", mustIndex(t, id, false, base, identity.ContentID{}, key, LensDynamic, 0, values, valuesID, 2)},
	}
	for _, test := range rows {
		t.Run(test.name, func(t *testing.T) {
			if !test.row.Available() || test.row.ID() != id || test.row.BaseSpan() != base {
				t.Fatal("available index row lost canonical geometry")
			}
			if test.row.Read() {
				if test.row.ResultSpan() != result || test.row.ValuesID().Available() {
					t.Fatal("read row exposed write geometry")
				}
			} else if span, position, ok := test.row.Values(); !ok || span != values || position != 2 || test.row.ResultSpan().Available() {
				t.Fatal("write row exposed malformed geometry")
			}
			if test.row.LensKind() == LensExact {
				if key, ok := test.row.ExactKey(); !ok || key != 7 || test.row.DynamicKeySpan().Available() {
					t.Fatal("exact lens accessors drifted")
				}
			} else if test.row.DynamicKeySpan() != key {
				t.Fatal("dynamic lens accessors drifted")
			}
		})
	}
}

func TestIndexRejectsMixedShapesAndFamilyPinsCatalog(t *testing.T) {
	id, base, result := indexLawID(t, "id"), indexLawID(t, "base"), indexLawID(t, "result")
	key, values, valuesID := indexLawID(t, "key"), indexLawID(t, "values"), indexLawID(t, "values-id")
	for _, test := range []struct {
		name             string
		read             bool
		result, key      identity.ContentID
		lens             uint8
		exact            uint64
		values, valuesID identity.ContentID
		position         int
	}{
		{"zero exact key", true, result, identity.ContentID{}, LensExact, 0, identity.ContentID{}, identity.ContentID{}, -1},
		{"dynamic no key", true, result, identity.ContentID{}, LensDynamic, 0, identity.ContentID{}, identity.ContentID{}, -1},
		{"read with values", true, result, key, LensDynamic, 0, values, valuesID, -1},
		{"write with result", false, result, key, LensDynamic, 0, values, valuesID, 0},
		{"write negative position", false, identity.ContentID{}, key, LensDynamic, 0, values, valuesID, -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if row, ok := NewIndex(id, test.read, base, test.result, test.key, test.lens, test.exact, test.values, test.valuesID, test.position); ok || row.Available() {
				t.Fatal("mixed index shape was accepted")
			}
		})
	}
	if Family().Definition() != programcatalog.HeapIndex() {
		t.Fatal("HeapIndex family drifted from the canonical catalog")
	}
}

func mustIndex(t *testing.T, id identity.ContentID, read bool, base, result, key identity.ContentID, lens uint8, exact uint64, values, valuesID identity.ContentID, position int) Index {
	t.Helper()
	row, ok := NewIndex(id, read, base, result, key, lens, exact, values, valuesID, position)
	if !ok {
		t.Fatal("valid index row refused")
	}
	return row
}
