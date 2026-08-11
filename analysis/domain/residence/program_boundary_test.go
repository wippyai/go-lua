package residence

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/keyspace"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func residenceBoundaryFixture(t testing.TB) *link.Link {
	t.Helper()
	p, err := lower.Lower(lower.Source{Name: "residence-boundary.lua", Text: []byte(`
local root = {}
local function retained() return root end
root.child = retained
return root
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "residence-boundary", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func TestSealOwnsExecutableProgramRetentionBoundaries(t *testing.T) {
	source := residenceBoundaryFixture(t)
	heapSchema, heapOK := heap.Seal(source)
	schema, ok := Seal(source, heapSchema)
	if !heapOK || !ok {
		t.Fatal("seal Residence schema")
	}
	want := map[BoundaryKind]int{}
	for shardIndex := 0; shardIndex < source.Project().Mounts().Count(); shardIndex++ {
		shard, _ := source.Project().Mounts().At(shardIndex)
		p, _ := source.Project().Mounts().Program(shard)
		flow := p.Flow()
		authored := flow.Authored()
		functions := authored.Functions()
		writes := authored.Storage().Writes()
		returns := authored.Control().Returns()
		executable := flow.Executable()
		exactLenses := authored.Access().Exact()
		dynamicLenses := authored.Access().Dynamic()
		for index := 0; index < functions.Count(); index++ {
			function, _ := functions.At(index)
			if !executable.Contains(function) {
				continue
			}
			captures, _ := functions.CaptureCount(function)
			for capture := 0; capture < captures; capture++ {
				if key, present := schema.KeyForCapture(shard, function, capture); !present || key.Kind() != BoundaryCapture {
				t.Fatalf("missing capture key %v:%d:%d", shard, function, capture)
				}
				want[BoundaryCapture]++
			}
		}
		for index := 0; index < writes.Count(); index++ {
			write, _ := writes.At(index)
			_, targetTerm, writeOK := writes.Get(write)
			_, _, _, _, exactLens := exactLenses.Get(targetTerm)
			_, _, _, dynamicLens := dynamicLenses.Get(targetTerm)
			if !writeOK || !executable.Contains(write) || (!exactLens && !dynamicLens) {
				continue
			}
			if key, present := schema.KeyForStore(shard, write); !present || key.Kind() != BoundaryStore {
				t.Fatalf("missing store key %v:%d", shard, write)
			}
			want[BoundaryStore]++
		}
		for index := 0; index < returns.Count(); index++ {
			term, _ := returns.At(index)
			if _, _, present := returns.Get(term); !present || !executable.Contains(term) {
				continue
			}
			if key, present := schema.KeyForReturn(shard, term); !present || key.Kind() != BoundaryReturn {
				t.Fatalf("missing return key %v:%d", shard, term)
			}
			want[BoundaryReturn]++
		}
	}
	got := map[BoundaryKind]int{}
	for index := 0; index < schema.KeyCount(); index++ {
		key, present := schema.KeyAt(index)
		if !present {
			t.Fatalf("missing key %d", index)
		}
		got[key.Kind()]++
	}
	for _, kind := range [...]BoundaryKind{BoundaryCapture, BoundaryStore, BoundaryReturn} {
		if got[kind] != want[kind] {
			t.Fatalf("%v boundaries=%d, want %d", kind, got[kind], want[kind])
		}
	}
	foreign, foreignOK := residenceBoundaryFixture(t).Project().Mounts().At(0)
	if !foreignOK {
		t.Fatal("foreign Project mount")
	}
	if _, present := schema.KeyForReturn(foreign, keyspace.Term(1)); present {
		t.Fatal("foreign Program coordinate entered Residence")
	}
}

func TestResidenceRequiresExactLiveHeapLinkAndRebindsColdIdentity(t *testing.T) {
	left := residenceBoundaryFixture(t)
	right := residenceBoundaryFixture(t)
	if left == right || left.ContentID() != right.ContentID() {
		t.Fatal("fixture must provide distinct same-content Link authorities")
	}
	leftHeap, leftHeapOK := heap.Seal(left)
	rightHeap, rightHeapOK := heap.Seal(right)
	if !leftHeapOK || !rightHeapOK || leftHeap.Link() != left || rightHeap.Link() != right || leftHeap.LinkContentID() != rightHeap.LinkContentID() {
		t.Fatal("fixture Heap authorities")
	}
	if _, ok := Seal(left, rightHeap); ok {
		t.Fatal("Residence accepted a same-content foreign Heap authority")
	}
	schema, ok := Seal(left, leftHeap)
	if !ok || !schema.valid() {
		t.Fatal("Residence rejected its local Heap authority")
	}
	rebound, ok := schema.Rebind(right)
	if !ok || !rebound.valid() || rebound.LinkContentID() != schema.LinkContentID() {
		t.Fatal("Residence failed cold rebind to an equivalent Link")
	}
	broken := *schema.owner
	broken.heap = rightHeap
	if (Schema{owner: &broken}).valid() {
		t.Fatal("Residence.valid accepted a privately inconsistent Heap/Link pair")
	}
}
