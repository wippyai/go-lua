package tuple_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

func TestInputPreservesOneReaderFiberAsOneTuple(t *testing.T) {
	fixture := testfixture.New(t, 1)
	reader, ok := fixture.ReaderLeftPayload(fixture.BothRoot())
	if !ok || !reader.Available() {
		t.Fatal("payload reader")
	}
	seen := 0
	completed, valid := reader.Scan(func(row read.Row) bool {
		value, inputOK := tuple.Input(fixture.Mounted(), reader, row)
		if !inputOK || !value.ValidFor(fixture.Mounted()) {
			t.Errorf("input tuple refused owned row")
			return false
		}
		if value.SourceLen() != 1 {
			t.Errorf("source count = %d, want one", value.SourceLen())
			return false
		}
		source, sourceOK := value.SourceAt(0)
		if !sourceOK || source != row.ID() || !value.Scope().Same(row.Scope()) || value.Lineage() != row.Lineage() {
			t.Errorf("tuple lost row identity, scope, or lineage")
			return false
		}
		cells, expected := value.Cells(), row.Cells()
		if len(cells) != len(expected) {
			t.Errorf("cell count = %d, want %d", len(cells), len(expected))
			return false
		}
		for index := range cells {
			if cells[index].Column() != expected[index].Column() || cells[index].Type() != expected[index].Type() || cells[index].Presence() != expected[index].Presence() || cells[index].Value().Available() != expected[index].Value().Available() || cells[index].Value().Available() && !cells[index].Value().Same(expected[index].Value()) {
				t.Errorf("cell %d did not retain the checked reader projection", index)
				return false
			}
		}
		seen++
		return true
	})
	if !completed || !valid || seen == 0 {
		t.Fatalf("reader scan = (%v, %v), tuples=%d", completed, valid, seen)
	}
}

func TestInputRefusesForeignMountAndDifferentReader(t *testing.T) {
	fixture := testfixture.New(t, 1)
	foreign := testfixture.New(t, 2)
	payload, ok := fixture.ReaderLeftPayload(fixture.BothRoot())
	if !ok {
		t.Fatal("payload reader")
	}
	keyed, ok := fixture.ReaderLeftKey(fixture.BothRoot())
	if !ok {
		t.Fatal("key reader")
	}
	var row read.Row
	completed, valid := payload.Scan(func(candidate read.Row) bool { row = candidate; return false })
	if completed || !valid || row == nil {
		t.Fatal("payload row")
	}
	if value, accepted := tuple.Input(foreign.Mounted(), payload, row); accepted || value.Available() {
		t.Fatal("foreign mounted runtime accepted tuple input")
	}
	if value, accepted := tuple.Input(fixture.Mounted(), keyed, row); accepted || value.Available() {
		t.Fatal("different reader accepted a borrowed row")
	}
}

func TestTupleHasNoCallbackOrIdentityMintingSurface(t *testing.T) {
	for _, value := range []reflect.Type{reflect.TypeOf(tuple.Tuple{}), reflect.TypeOf(tuple.Cell{})} {
		for index := 0; index < value.NumField(); index++ {
			field := value.Field(index)
			if field.PkgPath == "" {
				t.Fatalf("%s exposes mutable field %q", value, field.Name)
			}
			if field.Type.Kind() == reflect.Func {
				t.Fatalf("%s stores callback %q", value, field.Name)
			}
		}
	}
	var zero tuple.Tuple
	if zero.Available() || zero.ValidFor(testfixture.New(t, 3).Mounted()) || len(zero.Sources()) != 0 || len(zero.Cells()) != 0 {
		t.Fatal("zero tuple authenticated")
	}
}
