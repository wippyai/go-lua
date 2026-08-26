package column

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestColumnScanIsExtensionalToAscendingReads(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	left := newMask(t, fixture.guards, 1, true)
	right := newMask(t, fixture.guards, 1, false)
	absent, absentLineage := fixture.cell(t, "scan-absent", model.ProvenAbsent)
	absentUpdate, ok := NewUpdate(geometry.Key(2), scope, absent, absentLineage)
	if !ok {
		t.Fatal("construct explicit absence")
	}
	version, _, ok := fixture.base.Next(
		newUpdate(t, fixture, geometry.Key(7), scope, "scan-seven"),
		newUpdate(t, fixture, geometry.Key(0), right, "scan-zero-right"),
		newUpdate(t, fixture, geometry.Key(0), left, "scan-zero-left"),
		absentUpdate,
	)
	if !ok {
		t.Fatal("publish scan fixture")
	}
	expected := make([]ReadPart, 0, 4)
	for _, key := range []geometry.Key{0, 2, 7} {
		expected = append(expected, readParts(t, version, key, scope)...)
	}
	scratch := NewReadScratch(version.Guards())
	actual := make([]ReadPart, 0, len(expected))
	completed, valid := version.Scan(scope, scratch, func(part ReadPart) bool {
		actual = append(actual, part)
		return true
	})
	if !completed || !valid || len(actual) != len(expected) {
		t.Fatalf("scan count=(%d,%v,%v), read count=%d", len(actual), completed, valid, len(expected))
	}
	for index, want := range expected {
		got := actual[index]
		if got.Key() != want.Key() || !got.Region().Equal(want.Region()) || !got.Cell().SemanticSame(want.Cell()) || got.Lineage() != want.Lineage() {
			t.Fatalf("scan partition %d differs from Read", index)
		}
		if index > 0 && actual[index-1].Key() > got.Key() {
			t.Fatal("scan keys are not canonical ascending geometry keys")
		}
	}
	if len(actual) == 0 || actual[0].Key() != geometry.Key(0) || actual[len(actual)-1].Key() != geometry.Key(7) {
		t.Fatal("scan omitted committed key order")
	}
}

func TestColumnScanEarlyStopAndBorrowedPath(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	version, _, ok := fixture.base.Next(
		newUpdate(t, fixture, geometry.Key(1), scope, "scan-stop-one"),
		newUpdate(t, fixture, geometry.Key(2), scope, "scan-stop-two"),
	)
	if !ok {
		t.Fatal("publish scan stop fixture")
	}
	borrowed, ok := version.Borrow()
	if !ok {
		t.Fatal("borrow version")
	}
	scratch := NewReadScratch(version.Guards())
	visits := 0
	completed, valid := borrowed.Scan(scope, scratch, func(ReadPart) bool {
		visits++
		return false
	})
	if completed || !valid || visits != 1 {
		t.Fatalf("early stop=(%v,%v), visits=%d", completed, valid, visits)
	}
}

func TestColumnScanRejectsForeignMaskAndScratch(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	version, _, ok := fixture.base.Next(newUpdate(t, fixture, geometry.Key(4), scope, "scan-foreign"))
	if !ok {
		t.Fatal("publish scan foreign fixture")
	}
	foreignGuards, err := guard.New([]guard.Atom{1, 2})
	if err != nil {
		t.Fatal("construct foreign guards")
	}
	foreignScope := trueMask(t, foreignGuards)
	goodScratch := NewReadScratch(version.Guards())
	if completed, valid := version.Scan(foreignScope, goodScratch, func(ReadPart) bool { return true }); completed || valid {
		t.Fatal("foreign scan mask accepted")
	}
	foreignScratch := NewReadScratch(foreignGuards)
	if completed, valid := version.Scan(scope, foreignScratch, func(ReadPart) bool { return true }); completed || valid {
		t.Fatal("foreign scan scratch accepted")
	}
	if completed, valid := version.Scan(scope, goodScratch, nil); completed || valid {
		t.Fatal("nil scan visitor accepted")
	}
}

func TestColumnScanWarmCallerScratchAllocatesNothing(t *testing.T) {
	fixture := newLawFixture(t)
	scope := trueMask(t, fixture.guards)
	version, _, ok := fixture.base.Next(
		newUpdate(t, fixture, geometry.Key(0), scope, "scan-warm-zero"),
		newUpdate(t, fixture, geometry.Key(1), scope, "scan-warm-one"),
		newUpdate(t, fixture, geometry.Key(2), scope, "scan-warm-two"),
	)
	if !ok {
		t.Fatal("publish scan warm fixture")
	}
	scratch := NewReadScratch(version.Guards())
	scratch.semantic = make([]semanticPartition, 0, 2)
	scratch.lineage = make([]lineagePartition, 0, 2)
	visit := func(ReadPart) bool { return true }
	if completed, valid := version.Scan(scope, scratch, visit); !completed || !valid {
		t.Fatal("warm scan setup failed")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if completed, valid := version.Scan(scope, scratch, visit); !completed || !valid {
			t.Fatal("warm scan failed")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm scan allocated %v times", allocs)
	}
}
