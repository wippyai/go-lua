package event_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/relation/tuple/event"
	fixture "github.com/wippyai/go-lua/analysis/engine/testdata/relationfixture"
)

func TestBindMaterializesOrderedTupleEvents(t *testing.T) {
	world := fixture.New(t)
	delta, ok := world.BaseToLeftDelta()
	if !ok {
		t.Fatal("delta")
	}
	node, ok := world.LeftInputNode()
	if !ok {
		t.Fatal("input node")
	}
	authority, ok := node.Range()
	if !ok {
		t.Fatal("input range")
	}
	batch, ok := event.Bind(world.Mounted(), delta, world.LayoutLeftPayload(), authority, world.Geometry(), world.Scratch())
	if !ok || !batch.Available() || !batch.ValidFor(world.Mounted()) {
		t.Fatalf("batch=(%v,%v)", ok, batch.Available())
	}
	if batch.Len() != len(world.RowsLeft()) || !batch.Base().Same(delta.Base()) || !batch.Next().Same(delta.Next()) {
		t.Fatalf("batch len/base/next=%d/%v/%v", batch.Len(), batch.Base().Same(delta.Base()), batch.Next().Same(delta.Next()))
	}
	for index := 0; index < batch.Len(); index++ {
		value, ok := batch.At(index)
		if !ok || !value.Available() {
			t.Fatalf("event %d unavailable", index)
		}
		if after, present := value.After(); !present || !after.Available() {
			t.Fatalf("event %d missing successor", index)
		}
	}
}

func TestBindRejectsForeignAuthorities(t *testing.T) {
	left := fixture.New(t)
	foreign := fixture.New(t, 0x72)
	delta, ok := left.BaseToLeftDelta()
	if !ok {
		t.Fatal("delta")
	}
	node, ok := foreign.LeftInputNode()
	if !ok {
		t.Fatal("foreign input node")
	}
	authority, ok := node.Range()
	if !ok {
		t.Fatal("foreign range")
	}
	if batch, ok := event.Bind(left.Mounted(), delta, left.LayoutLeftPayload(), authority, left.Geometry(), left.Scratch()); ok || batch.Available() {
		t.Fatal("foreign range redeemed")
	}
}

func TestBatchValidForIsRepeatedZeroAllocation(t *testing.T) {
	world := fixture.New(t)
	delta, ok := world.BaseToLeftDelta()
	if !ok {
		t.Fatal("delta")
	}
	node, ok := world.LeftInputNode()
	if !ok {
		t.Fatal("input node")
	}
	authority, ok := node.Range()
	if !ok {
		t.Fatal("input range")
	}
	batch, ok := event.Bind(world.Mounted(), delta, world.LayoutLeftPayload(), authority, world.Geometry(), world.Scratch())
	if !ok || !batch.Available() {
		t.Fatal("bind")
	}
	if allocations := testing.AllocsPerRun(100, func() { if !batch.ValidFor(world.Mounted()) { t.Fatal("invalid batch") } }); allocations != 0 {
		t.Fatalf("ValidFor allocated %v times per run", allocations)
	}
}
