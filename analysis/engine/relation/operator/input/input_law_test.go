package input

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/arrangement"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestInputMaterializesCompleteAuthoredRowVector(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := inputBinding(t, fixture, fixture.LayoutInput())
	reader, ok := read.Bind(fixture.LeftRoot(), binding.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("relation Input reader refused")
	}
	batches, ok := Execute(binding, fixture.Mounted(), reader)
	if !ok || len(batches) != 1 || !batches[0].Available() {
		t.Fatalf("Input batches=(%v,%d)", ok, len(batches))
	}
	batch := batches[0]
	fiber, _ := fixture.OverlapScopes()
	count := 0
	if !batch.ValidFor(fixture.Mounted()) || !batch.Scope().Same(fiber) {
		t.Fatal("invalid Input batch")
	}
	for index := 0; index < batch.Len(); index++ {
		value, valueOK := batch.At(index)
		if !valueOK || value.SourceLen() != 1 || value.Len() != len(binding.Values().Columns()) {
			t.Fatal("Input lost its sealed authored row vector")
		}
		for _, column := range binding.Values().Columns() {
			if _, cellOK := value.CellFor(column); !cellOK {
				t.Fatalf("Input omitted sealed column %v", column)
			}
		}
		count++
	}
	if count != len(fixture.RowsLeft()) {
		t.Fatalf("Input rows=%d, want=%d", count, len(fixture.RowsLeft()))
	}
}

func TestInputSealsAuthoredOrderAndRefusesRangeScanAsValues(t *testing.T) {
	fixture := testfixture.New(t, 0x71)
	binding := inputBinding(t, fixture, fixture.LayoutInput())
	want := []model.ColumnID{
		fixture.KeyColumnsLeft()[0], fixture.KeyColumnsLeft()[1],
		fixture.PayloadColumnsLeft()[0], fixture.PayloadColumnsLeft()[1],
	}
	if !reflect.DeepEqual(binding.Values().Columns(), want) {
		t.Fatalf("Input values = %v, want full authored order %v", binding.Values().Columns(), want)
	}
	if scan := binding.Scan(); scan.Access().Key().Available() || len(scan.Columns()) != 0 {
		t.Fatal("Input range authority was not the relation cofiber")
	}
	reader, ok := read.Bind(fixture.LeftRoot(), binding.Scan(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("scan reader")
	}
	if batches, accepted := Execute(binding, fixture.Mounted(), reader); accepted || batches != nil {
		t.Fatal("Input accepted a range scan as a replacement values vector")
	}
}

func inputBinding(t testing.TB, fixture testfixture.Fixture, layout arrangement.Layout) arrangement.InputBinding {
	t.Helper()
	// InputBinding is sealed by arrangement.Execution. The fixture currently
	// exposes only its immutable plan, so this law obtains the binding from the
	// exact execution entry once the Input expression is included in the mount
	// certificate. It deliberately refuses a hand-constructed binding.
	for _, id := range fixture.Mounted().Arrangement().Execution().ExpressionIDs() {
		if node, ok := fixture.Mounted().Arrangement().Execution().Entry(id); ok {
			if value, ok := node.Input(); ok && value.Scan().Equal(layout) {
				return value
			}
		}
	}
	t.Fatalf("missing sealed InputBinding")
	return arrangement.InputBinding{}
}
