package store

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
	"github.com/wippyai/go-lua/analysis/engine/relation/state/geometry"
	"github.com/wippyai/go-lua/analysis/relation/schema/model"
)

func TestReadAndScanAreTheSameCanonicalProjection(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	_, delta := lawSuccessor(t, fixture, 0, "read-door")
	version, _, ok := commit(base, delta)
	if !ok || !version.Available() {
		t.Fatal("publish read fixture")
	}
	within := lawMask(t, fixture)
	scratch := NewReadScratch(fixture.guards)
	if scratch == nil || !scratch.Available() {
		t.Fatal("construct read scratch")
	}
	var read []ReadPart
	completed, valid := version.Read(fixture.columns[0], geometry.Key(0), within, scratch, func(part ReadPart) bool {
		read = append(read, part)
		return true
	})
	if !completed || !valid || len(read) != 1 {
		t.Fatalf("read=(%d,%v,%v)", len(read), completed, valid)
	}
	part := read[0]
	if part.Key() != geometry.Key(0) || !part.Region().Equal(within) || part.Column() != fixture.columns[0] || part.Type() != fixture.types[0] || !part.Presence().Is(model.Present) || !part.Value().Available() || !part.Lineage().Available() {
		t.Fatal("read projection lost authenticated semantic fields")
	}

	var scan []ReadPart
	completed, valid = version.Scan(fixture.columns[0], within, scratch, func(part ReadPart) bool {
		scan = append(scan, part)
		return true
	})
	if !completed || !valid || len(scan) != len(read) {
		t.Fatalf("scan=(%d,%v,%v), read=%d", len(scan), completed, valid, len(read))
	}
	for index := range scan {
		if scan[index].Key() != read[index].Key() || !scan[index].Region().Equal(read[index].Region()) || scan[index].Value() != read[index].Value() || scan[index].Presence() != read[index].Presence() || scan[index].Lineage() != read[index].Lineage() {
			t.Fatalf("scan/read mismatch at %d", index)
		}
	}
}

func TestReadDoorRejectsForeignAuthorityAndPreservesEarlyStop(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	_, delta := lawSuccessor(t, fixture, 0, "read-fence")
	version, _, ok := commit(base, delta)
	if !ok {
		t.Fatal("publish read fence fixture")
	}
	within := lawMask(t, fixture)
	scratch := NewReadScratch(fixture.guards)
	if completed, valid := version.Read(fixture.columns[0], geometry.Key(0), within, scratch, func(ReadPart) bool { return false }); completed || !valid {
		t.Fatal("callback early stop was not preserved")
	}
	foreign, err := guard.New([]guard.Atom{9, 10})
	if err != nil {
		t.Fatal("construct foreign guards")
	}
	foreignMask, ok := support.True(foreign)
	if !ok {
		t.Fatal("construct foreign mask")
	}
	if completed, valid := version.Scan(fixture.columns[0], foreignMask, scratch, func(ReadPart) bool { return true }); completed || valid {
		t.Fatal("foreign support authority accepted")
	}
	foreignScratch := NewReadScratch(foreign)
	if completed, valid := version.Read(fixture.columns[0], geometry.Key(0), within, foreignScratch, func(ReadPart) bool { return true }); completed || valid {
		t.Fatal("foreign scratch authority accepted")
	}
	if completed, valid := version.Scan(fixture.columns[0], within, scratch, nil); completed || valid {
		t.Fatal("nil visitor accepted")
	}
}

func TestReadDoorWarmScanHasNoAllocations(t *testing.T) {
	fixture := newLawFixture(t)
	base := lawAggregate(t, fixture)
	_, delta := lawSuccessor(t, fixture, 0, "read-warm")
	version, _, ok := commit(base, delta)
	if !ok {
		t.Fatal("publish warm fixture")
	}
	within := lawMask(t, fixture)
	scratch := NewReadScratch(fixture.guards)
	visit := func(ReadPart) bool { return true }
	if completed, valid := version.Scan(fixture.columns[0], within, scratch, visit); !completed || !valid {
		t.Fatal("warm scan setup")
	}
	allocs := testing.AllocsPerRun(100, func() {
		if completed, valid := version.Scan(fixture.columns[0], within, scratch, visit); !completed || !valid {
			t.Fatal("warm scan failed")
		}
	})
	if allocs != 0 {
		t.Fatalf("warm scan allocated %v times", allocs)
	}
}
