package localtransfer

import (
	"bytes"
	"slices"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
	"github.com/wippyai/go-lua/analysis/schema"
)

func testID(value byte) identity.ContentID {
	var id identity.ContentID
	id[0] = value
	return id
}

func TestAppendCanonicalizesWriteIdentity(t *testing.T) {
	from, to := testID(1), testID(2)
	left := New(17)
	right := New(17)
	if !left.Append("transfer", from, to, false, "value-source", "pack-source", "heap-ingress", "call-dispatch") ||
		!right.Append("transfer", from, to, false, "call-dispatch", "heap-ingress", "pack-source", "value-source") {
		t.Fatal("valid factor transport was rejected")
	}
	if left.Append("transfer", from, to, false, "value-source", "value-source") {
		t.Fatal("duplicate write key was accepted")
	}
	if fault := left.Seal(); fault.Failed() {
		t.Fatalf("left seal fault=%#v", fault)
	}
	if fault := right.Seal(); fault.Failed() {
		t.Fatalf("right seal fault=%#v", fault)
	}
	leftRows, leftWrites, leftOK := left.TakeCanonicalPlanes()
	rightRows, rightWrites, rightOK := right.TakeCanonicalPlanes()
	if !leftOK || !rightOK || len(leftRows) != 1 || len(rightRows) != 1 {
		t.Fatalf("canonical planes=%d/%d/%t/%t", len(leftRows), len(rightRows), leftOK, rightOK)
	}
	if leftRows[0].ID() != rightRows[0].ID() || !slices.Equal(leftWrites, rightWrites) {
		t.Fatal("set-identical writes changed canonical identity or plane")
	}
	if count := leftRows[0].WritesCount(); count != 4 {
		t.Fatalf("write count=%d, want 4", count)
	}
	offset, count, spanOK := leftRows[0].WriteSpan()
	if !spanOK || offset != 0 || count != 4 {
		t.Fatalf("write span=%d/%d/%t, want 0/4/true", offset, count, spanOK)
	}
	for index, want := range []schema.Key{"call-dispatch", "heap-ingress", "pack-source", "value-source"} {
		got, ok := leftWrites[index].Key()
		if !ok || got != want {
			t.Fatalf("write[%d]=%q/%t, want %q", index, got, ok, want)
		}
	}
}

func TestSealSortsFromToIDAndRejectsDuplicate(t *testing.T) {
	owner := New(23)
	for index, row := range []struct {
		from, to byte
		domain   string
	}{
		{from: 9, to: 1, domain: "third"},
		{from: 1, to: 9, domain: "first"},
		{from: 1, to: 9, domain: "second"},
	} {
		if !owner.Append(row.domain, testID(row.from), testID(row.to), true) {
			t.Fatalf("append[%d] rejected", index)
		}
	}
	if fault := owner.Seal(); fault.Failed() {
		t.Fatalf("seal fault=%#v", fault)
	}
	rows, _, ok := owner.TakeCanonicalPlanes()
	if !ok || len(rows) != 3 {
		t.Fatalf("canonical rows=%d/%t", len(rows), ok)
	}
	for index := 1; index < len(rows); index++ {
		prior, current := rows[index-1], rows[index]
		priorFrom, currentFrom := prior.From(), current.From()
		priorTo, currentTo := prior.To(), current.To()
		priorID, currentID := prior.ID(), current.ID()
		if bytes.Compare(priorFrom[:], currentFrom[:]) > 0 ||
			(priorFrom == currentFrom && bytes.Compare(priorTo[:], currentTo[:]) > 0) ||
			(priorFrom == currentFrom && priorTo == currentTo && bytes.Compare(priorID[:], currentID[:]) >= 0) {
			t.Fatalf("rows are not in From/To/ID order at %d", index)
		}
	}

	duplicate := New(23)
	if !duplicate.Append("same", testID(1), testID(2), true) || !duplicate.Append("same", testID(1), testID(2), true) {
		t.Fatal("duplicate fixture append failed")
	}
	fault := duplicate.Seal()
	if !fault.Failed() || fault.Index() != 1 {
		t.Fatalf("duplicate fault=%#v, want failed index 1", fault)
	}
	if _, _, ok := duplicate.TakeCanonicalPlanes(); ok {
		t.Fatal("duplicate owner transferred canonical planes")
	}
}

func TestTakeCanonicalPlanesTransfersExactlyOnce(t *testing.T) {
	owner := New(29)
	if !owner.Append("full", testID(1), testID(2), true) || !owner.Append("factor", testID(2), testID(3), false, "pack-source") {
		t.Fatal("valid transport fixture append failed")
	}
	if fault := owner.Seal(); fault.Failed() {
		t.Fatalf("seal fault=%#v", fault)
	}
	transfers, writes, ok := owner.TakeCanonicalPlanes()
	if !ok || len(transfers) != 2 || len(writes) != 1 {
		t.Fatalf("transferred planes=%d/%d/%t", len(transfers), len(writes), ok)
	}
	if _, _, ok := owner.TakeCanonicalPlanes(); ok {
		t.Fatal("canonical planes transferred twice")
	}
	for _, transfer := range transfers {
		if !transfer.Available() {
			t.Fatal("canonical transfer unavailable")
		}
	}
}
