package evaluation

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/keyspace"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestSealPendingProductionMatrixCoversSixSubjectPlanesAndTenBuckets(t *testing.T) {
	counts := pendingRuntimeMatrixCounts()
	body := pendingTerm(keyspace.FamilyBody, 1)
	fixture := openPendingFixture(t, "pending-runtime-buckets.lua", counts,
		pendingRuntimeMatrixRows(), pendingRuntimeMatrixFlow(), []source.BindCells{
			{Bind: pendingTerm(keyspace.FamilyBind, 1), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 2)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 2), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 3)}},
			{Bind: pendingTerm(keyspace.FamilyBind, 3), Cells: []keyspace.Term{pendingTerm(keyspace.FamilyCell, 4)}},
		}, nil, nil, pendingSourceExtras{
			keys: []source.KeyInput{source.NameKey(body, "field-list"), source.NameKey(body, "field-name"), source.NameKey(body, "method")},
			exactAtoms: []keyspace.LiteralValue{
				{Kind: keyspace.LiteralString, String: "field-list"},
				{Kind: keyspace.LiteralString, String: "field-name"},
				{Kind: keyspace.LiteralString, String: "method"},
			},
		})
	for _, bucket := range []struct {
		name     string
		contains func(keyspace.Term) bool
		subject  keyspace.Term
	}{
		{"UnaryNumeric", fixture.candidates.UnaryNumeric().Contains, pendingTerm(keyspace.FamilyUnary, 1)},
		{"Length", fixture.candidates.Length().Contains, pendingTerm(keyspace.FamilyUnary, 2)},
		{"Arithmetic", fixture.candidates.Arithmetic().Contains, pendingTerm(keyspace.FamilyBinary, 1)},
		{"Bitwise", fixture.candidates.Bitwise().Contains, pendingTerm(keyspace.FamilyBinary, 2)},
		{"Concat", fixture.candidates.Concat().Contains, pendingTerm(keyspace.FamilyBinary, 3)},
		{"Equality", fixture.candidates.Equality().Contains, pendingTerm(keyspace.FamilyBinary, 4)},
		{"Order", fixture.candidates.Order().Contains, pendingTerm(keyspace.FamilyBinary, 5)},
		{"IndexGet", fixture.candidates.IndexGet().Contains, pendingTerm(keyspace.FamilyRead, 1)},
		{"IndexSet", fixture.candidates.IndexSet().Contains, pendingTerm(keyspace.FamilyWrite, 2)},
	} {
		if !bucket.contains(bucket.subject) {
			t.Fatalf("candidate bucket %s did not contain %v", bucket.name, bucket.subject)
		}
	}
	loopFixture := openPendingLoopFixture(t, "pending-control-bucket.lua")
	if !loopFixture.candidates.GenericLoop().Contains(pendingTerm(keyspace.FamilyLoop, 4)) {
		t.Fatal("candidate bucket GenericLoop did not contain the fixed-header GenericFor")
	}
	if !fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 2)) ||
		!fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 3)) ||
		!fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 4)) {
		t.Fatal("dynamic IndexGet/IndexSet rows were not retained")
	}
	if !fixture.candidates.UnaryNumeric().Contains(pendingTerm(keyspace.FamilyUnary, 3)) {
		t.Fatal("runtime FieldExact UnaryNeg was not classified as a live numeric candidate")
	}
	binaryOne := pendingTerm(keyspace.FamilyBinary, 1)
	binaryOneCount, binaryOneOK := fixture.pending.Count(binaryOne)
	if !binaryOneOK || binaryOneCount == 0 {
		t.Fatal("table allocation did not precede the first Binary pending boundary")
	}
	var sawTableAllocation bool
	for index := 0; index < binaryOneCount; index++ {
		value, valueOK := fixture.pending.At(binaryOne, index)
		if !valueOK {
			t.Fatalf("Binary1 pending At(%d) was unavailable", index)
		}
		sawTableAllocation = sawTableAllocation || value == pendingTerm(keyspace.FamilyTable, 1)
	}
	if !sawTableAllocation {
		t.Fatal("table allocation was not retained before the first Binary")
	}
	if fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 3)) ||
		fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 4)) ||
		fixture.candidates.IndexGet().Contains(pendingTerm(keyspace.FamilyRead, 5)) ||
		fixture.candidates.IndexSet().Contains(pendingTerm(keyspace.FamilyWrite, 1)) {
		t.Fatal("static/cell read-write rows entered candidate buckets")
	}
	for _, subject := range []keyspace.Term{
		pendingTerm(keyspace.FamilyUnary, 1), pendingTerm(keyspace.FamilyUnary, 2), pendingTerm(keyspace.FamilyUnary, 3),
		pendingTerm(keyspace.FamilyBinary, 1), pendingTerm(keyspace.FamilyBinary, 2), pendingTerm(keyspace.FamilyBinary, 3),
		pendingTerm(keyspace.FamilyBinary, 4), pendingTerm(keyspace.FamilyBinary, 5),
		pendingTerm(keyspace.FamilyRead, 1), pendingTerm(keyspace.FamilyRead, 2),
		pendingTerm(keyspace.FamilyWrite, 2), pendingTerm(keyspace.FamilyWrite, 3), pendingTerm(keyspace.FamilyWrite, 4),
		pendingTerm(keyspace.FamilyCall, 1), pendingTerm(keyspace.FamilyCall, 2), pendingTerm(keyspace.FamilyCall, 3),
	} {
		_, ok := fixture.pending.Count(subject)
		if !ok {
			t.Fatalf("production candidate subject %v was not admitted", subject)
		}
	}
	methodRead := pendingTerm(keyspace.FamilyRead, 2)
	methodReadCount, methodReadOK := fixture.pending.Count(methodRead)
	if !methodReadOK || methodReadCount < 2 {
		t.Fatalf("method actual Read pending = %d/%v, want callee and receiver prefix", methodReadCount, methodReadOK)
	}
	var sawMethodCallee, sawMethodReceiver bool
	for index := 0; index < methodReadCount; index++ {
		value, valueOK := fixture.pending.At(methodRead, index)
		if !valueOK {
			t.Fatalf("method actual Read At(%d) was unavailable", index)
		}
		sawMethodCallee = sawMethodCallee || value == pendingTerm(keyspace.FamilyRead, 1)
		sawMethodReceiver = sawMethodReceiver || value == pendingTerm(keyspace.FamilyRead, 6)
	}
	if !sawMethodCallee || !sawMethodReceiver {
		t.Fatalf("method prefix omitted callee/receiver: callee=%v receiver=%v", sawMethodCallee, sawMethodReceiver)
	}
	if _, tail, ok := fixture.flowView.Values().Get(pendingTerm(keyspace.FamilyValues, 5)); !ok || tail != pendingTerm(keyspace.FamilyVararg, 1) || !fixture.executable.Executable(tail) {
		t.Fatal("method actual Values tail was not retained as the authored open tail")
	}
	if fieldCount, ok := fixture.flowView.Tables().FieldCount(pendingTerm(keyspace.FamilyTable, 1)); !ok || fieldCount != 4 {
		t.Fatalf("table field count = %d/%v, want four ordered fields", fieldCount, ok)
	}
	for index := 0; index < 4; index++ {
		field, ok := fixture.flowView.Tables().FieldAt(pendingTerm(keyspace.FamilyTable, 1), index)
		if !ok || field != pendingTerm(keyspace.FamilyTableField, uint32(index+1)) {
			t.Fatalf("table FieldAt(%d) = %v/%v, want Field%d/true", index, field, ok, index+1)
		}
	}
	assign := pendingTerm(keyspace.FamilyAssign, 1)
	writeCount, writeCountOK := fixture.flowView.Storage().Assigns().WriteCount(assign)
	if !writeCountOK || writeCount != 4 {
		t.Fatalf("assignment write count = %d/%v, want four target-ordered writes", writeCount, writeCountOK)
	}
	wantTargets := []keyspace.Term{
		pendingTerm(keyspace.FamilyCell, 2), pendingTerm(keyspace.FamilyLensExact, 2),
		pendingTerm(keyspace.FamilyLensKey, 2), pendingTerm(keyspace.FamilyLensExact, 3),
	}
	for index, wantTarget := range wantTargets {
		write, ok := fixture.flowView.Storage().Assigns().WriteAt(assign, index)
		if !ok {
			t.Fatalf("assignment WriteAt(%d) unavailable", index)
		}
		_, target, ok := fixture.flowView.Storage().Writes().Get(write)
		if !ok || target != wantTarget {
			t.Fatalf("assignment target %d = %v/%v, want %v/true", index, target, ok, wantTarget)
		}
	}
	call2 := pendingTerm(keyspace.FamilyCall, 2)
	call2Count, call2OK := fixture.pending.Count(call2)
	if !call2OK || call2Count == 0 {
		t.Fatal("nested Call after the guarded Select did not receive a nonempty prefix")
	}
	for index := 0; index < call2Count; index++ {
		value, valueOK := fixture.pending.At(call2, index)
		if !valueOK || value == pendingTerm(keyspace.FamilyNil, 17) || value == pendingTerm(keyspace.FamilyNil, 18) {
			t.Fatalf("guarded Select operand leaked into later Call prefix at %d: %v/%v", index, value, valueOK)
		}
	}
	for _, absent := range []keyspace.Term{
		pendingTerm(keyspace.FamilyRead, 3), pendingTerm(keyspace.FamilyRead, 4), pendingTerm(keyspace.FamilyRead, 5), pendingTerm(keyspace.FamilyRead, 6), pendingTerm(keyspace.FamilyWrite, 1),
		pendingTerm(keyspace.FamilyLoop, 1),
		pendingTerm(keyspace.FamilyKey, 1),
	} {
		if _, ok := fixture.pending.Count(absent); ok {
			t.Fatalf("noncandidate subject %v was admitted", absent)
		}
	}
}
