package input

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/state/read"
	"github.com/wippyai/go-lua/analysis/engine/relation/tuple"
	relationfixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
	"github.com/wippyai/go-lua/analysis/relation/mount/witness"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
	bindingpkg "github.com/wippyai/go-lua/analysis/relation/semantic/binding"
)

func TestExecuteExtentABI(t *testing.T) {
	typ := reflect.TypeOf(ExecuteExtent)
	if typ.NumIn() != 6 || typ.In(4) != reflect.TypeOf([]model.RowID{}) || typ.In(5) != reflect.TypeOf(witness.Scope{}) || typ.NumOut() != 2 {
		t.Fatalf("Input extent ABI = %v", typ)
	}
}

func TestExecuteExtentUsesExactOrderedWitnessRows(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	node, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left Input node")
	}
	input, ok := node.Input()
	if !ok || !input.Available() {
		t.Fatal("left Input binding")
	}
	reader, ok := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("left Input reader")
	}
	ref, ok := model.NewDenominatorRef(fixture.RelationLeft(), fixture.KeyLeft())
	if !ok {
		t.Fatal("left denominator ref")
	}
	source, ok := fixture.Mounted().Denominator(ref)
	if !ok || !source.Available() {
		t.Fatal("left source witness")
	}
	scope, _ := fixture.OverlapScopes()
	rows := make([]model.RowID, source.Len())
	for index := range rows {
		rows[index], ok = source.At(index)
		if !ok {
			t.Fatal("source row")
		}
	}
	batches, ok := ExecuteExtent(input, fixture.Mounted(), reader, source, rows, scope)
	if !ok || len(batches) != 1 || !batches[0].ValidFor(fixture.Mounted()) {
		t.Fatalf("extent=(%v,%d)", ok, len(batches))
	}
	for index := range rows {
		value, valueOK := batches[0].At(index)
		if !valueOK {
			t.Fatalf("extent tuple %d", index)
		}
		got, rowOK := value.SourceAt(0)
		if !rowOK || got != rows[index] {
			t.Fatalf("extent row %d=%v want=%v", index, got, rows[index])
		}
	}

	if reversed := []model.RowID{rows[1], rows[0]}; func() bool {
		got, accepted := ExecuteExtent(input, fixture.Mounted(), reader, source, reversed, scope)
		return accepted || got != nil
	}() {
		t.Fatal("extent accepted reordered witness rows")
	}
	if duplicate := []model.RowID{rows[0], rows[0]}; func() bool {
		got, accepted := ExecuteExtent(input, fixture.Mounted(), reader, source, duplicate, scope)
		return accepted || got != nil
	}() {
		t.Fatal("extent accepted duplicate witness row")
	}
	if missing := rows[:len(rows)-1]; func() bool {
		got, accepted := ExecuteExtent(input, fixture.Mounted(), reader, source, missing, scope)
		return accepted || got != nil
	}() {
		t.Fatal("extent accepted incomplete witness rows")
	}
	foreign, foreignOK := model.IssueRowID(fixture.RelationRight(), rows[0].Content())
	if !foreignOK {
		t.Fatal("foreign extent row")
	}
	if foreignRows := []model.RowID{foreign, rows[1]}; func() bool {
		got, accepted := ExecuteExtent(input, fixture.Mounted(), reader, source, foreignRows, scope)
		return accepted || got != nil
	}() {
		t.Fatal("extent accepted foreign witness row")
	}
}

func TestExecuteExtentEmptyWitnessRetainsInputRange(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	node, ok := fixture.EmptyInputNode()
	if !ok {
		t.Fatal("empty Input node")
	}
	input, ok := node.Input()
	if !ok || !input.Available() {
		t.Fatal("empty Input binding")
	}
	reader, ok := read.Bind(fixture.Base(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("empty Input reader")
	}
	var source model.DenominatorRef
	for _, candidate := range fixture.Mounted().Denominators() {
		if candidate.Relation() != input.Relation() {
			continue
		}
		resolved, resolvedOK := fixture.Mounted().Denominator(candidate)
		if resolvedOK && resolved.Len() == 0 {
			source = candidate
			break
		}
	}
	if !source.Available() {
		t.Fatal("empty source denominator ref")
	}
	resolved, ok := fixture.Mounted().Denominator(source)
	if !ok || !resolved.Available() || resolved.Len() != 0 {
		t.Fatal("empty source witness")
	}
	scope, _ := fixture.OverlapScopes()
	batches, ok := ExecuteExtentFromWitness(input, fixture.Mounted(), reader, resolved, scope)
	if !ok || len(batches) != 1 || !batches[0].ValidFor(fixture.Mounted()) || batches[0].Len() != 0 || !batches[0].Scope().Same(scope) {
		t.Fatalf("empty extent=(%v,%d)", ok, len(batches))
	}
}

func TestExecuteRowABI(t *testing.T) {
	typ := reflect.TypeOf(ExecuteRow)
	if typ.NumIn() != 6 || typ.In(4) != reflect.TypeOf(model.RowID{}) || typ.In(5) != reflect.TypeOf(witness.Scope{}) || typ.NumOut() != 2 || typ.Out(0) != reflect.TypeOf(tuple.Batch{}) {
		t.Fatalf("Input population-row ABI = %v", typ)
	}
}

func TestExecuteRowSelectsExactScope(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	node, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left Input node")
	}
	input, ok := node.Input()
	if !ok || !input.Available() {
		t.Fatal("left Input binding")
	}
	reader, ok := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok || !reader.Available() {
		t.Fatal("left Input reader")
	}
	ref, ok := model.NewDenominatorRef(fixture.RelationLeft(), fixture.KeyLeft())
	if !ok {
		t.Fatal("left denominator ref")
	}
	source, ok := fixture.Mounted().Denominator(ref)
	if !ok || !source.Available() {
		t.Fatal("left source witness")
	}
	rowID, ok := source.At(0)
	if !ok {
		t.Fatal("left source row")
	}
	scope, _ := fixture.OverlapScopes()
	batch, ok := ExecuteRow(input, fixture.Mounted(), reader, source, rowID, scope)
	if !ok || !batch.ValidFor(fixture.Mounted()) || batch.Len() != 1 || !batch.Scope().Same(scope) {
		t.Fatalf("population row=(%v,%v,%d)", ok, batch.Available(), batch.Len())
	}
	value, ok := batch.At(0)
	if !ok || !value.ValidFor(fixture.Mounted()) {
		t.Fatal("population tuple")
	}
	got, ok := value.SourceAt(0)
	if !ok || got != rowID {
		t.Fatalf("population row=%v want=%v", got, rowID)
	}
	_, wrongScope := fixture.OverlapScopes()
	if wrong, accepted := ExecuteRow(input, fixture.Mounted(), reader, source, rowID, wrongScope); accepted || wrong.Available() {
		t.Fatal("population row accepted a different cofiber")
	}
}

func TestExecuteRowRejectsHostileIngress(t *testing.T) {
	fixture := relationfixture.New(t, 0x71)
	node, ok := fixture.LeftInputNode()
	if !ok {
		t.Fatal("left Input node")
	}
	input, ok := node.Input()
	if !ok {
		t.Fatal("left Input binding")
	}
	reader, ok := read.Bind(fixture.BothRoot(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("left Input reader")
	}
	ref, _ := model.NewDenominatorRef(fixture.RelationLeft(), fixture.KeyLeft())
	source, ok := fixture.Mounted().Denominator(ref)
	if !ok {
		t.Fatal("left source witness")
	}
	rowID, _ := source.At(0)
	scope, _ := fixture.OverlapScopes()
	foreign, _ := model.IssueRowID(fixture.RelationRight(), rowID.Content())
	if batch, accepted := ExecuteRow(input, fixture.Mounted(), reader, source, foreign, scope); accepted || batch.Available() {
		t.Fatal("population row accepted foreign RowID")
	}
	if batch, accepted := ExecuteRow(input, fixture.Mounted(), reader, source, rowID, witness.Scope{}); accepted || batch.Available() {
		t.Fatal("population row accepted unavailable scope")
	}
	if batch, accepted := ExecuteRow(input, fixture.Mounted(), reader, bindingpkg.DenominatorWitness{}, rowID, scope); accepted || batch.Available() {
		t.Fatal("population row accepted unavailable source witness")
	}
	baseReader, ok := read.Bind(fixture.Base(), input.Values(), fixture.Geometry(), fixture.Scratch())
	if !ok {
		t.Fatal("base Input reader")
	}
	if batch, accepted := ExecuteRow(input, fixture.Mounted(), baseReader, source, rowID, scope); accepted || batch.Available() {
		t.Fatal("population row accepted a missing physical row")
	}
}
