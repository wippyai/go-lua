package escape

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/analysis/domain/materialization"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	proglink "github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func escapeFixture(t *testing.T, name string) *proglink.Link {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: name + ".lua", Text: []byte(`
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
	source, err := proglink.Seal(&proglink.Spec{Target: contract, Modules: []linkproject.Module{{Name: name, Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func escapeSchema(t *testing.T, source *proglink.Link) (Schema, heap.Schema) {
	t.Helper()
	heapSchema, ok := heap.Seal(source)
	if !ok {
		t.Fatal("Heap schema")
	}
	schema, ok := NewSchema(source, heapSchema)
	if !ok {
		t.Fatal("Escape schema")
	}
	return schema, heapSchema
}

func TestEscapeSealsFiniteBoundaryKeysAndRootRelation(t *testing.T) {
	source := escapeFixture(t, "escape_owner")
	schema, heapSchema := escapeSchema(t, source)
	if !schema.Valid() || !heapSchema.Valid() || schema.CoordinateCount() == 0 || schema.RootCount() == 0 {
		t.Fatal("seal finite Escape universe")
	}
	coordinate, ok := schema.CoordinateAt(0)
	if !ok || !coordinate.Valid() {
		t.Fatal("first sealed boundary")
	}
	root, rawOK := heapSchema.KeyAt(0)
	if !rawOK {
		t.Fatal("source root")
	}
	admittedRoot, ok := schema.RootFor(root)
	if !ok {
		t.Fatal("sealed root")
	}
	recent, ok := schema.Of(admittedRoot, materialization.Recent)
	if !ok || !schema.Admit(coordinate, recent) {
		t.Fatal("recent root relation")
	}
	if _, ok := schema.RootFor(heap.Key{}); ok {
		t.Fatal("fabricated allocation root entered sealed support")
	}
	summary, _ := schema.Of(admittedRoot, materialization.Summary)
	joined, ok := schema.Join(recent, summary)
	if !ok || joined.Count() != 2 {
		t.Fatal("recent and summary alternatives must remain distinct")
	}
	if _, role, ok := joined.At(1); !ok || role != materialization.Summary {
		t.Fatal("relation traversal")
	}
}

func TestEscapeLawsRankAndSchemaFence(t *testing.T) {
	source := escapeFixture(t, "escape_laws")
	schema, heapSchema := escapeSchema(t, source)
	coordinate, _ := schema.CoordinateAt(0)
	root, _ := schema.RootAt(0)
	bottom, _ := schema.Bottom()
	recent, _ := schema.Of(root, materialization.Recent)
	summary, _ := schema.Of(root, materialization.Summary)
	top, _ := schema.Top()
	domain, ok := schema.Lattice()
	if !ok {
		t.Fatal("lattice")
	}
	latticelaws.LawSuite[Value]{Name: "escape", Domain: domain, Sample: []Value{bottom, recent, summary, domain.Join(recent, summary), top}}.Run(t)
	rank, _ := schema.WidenRank()
	first, firstOK := rank.At(coordinate, bottom, 0)
	second, secondOK := rank.At(coordinate, recent, 0)
	third, thirdOK := rank.At(coordinate, top, 0)
	if !firstOK || !secondOK || !thirdOK || !(first > second && second > third) {
		t.Fatalf("non-descending rank: %d %d %d", first, second, third)
	}
	firstFingerprint, firstFingerprintOK := schema.Fingerprint(domain.Join(recent, summary))
	secondFingerprint, secondFingerprintOK := schema.Fingerprint(domain.Join(summary, recent))
	if !firstFingerprintOK || !secondFingerprintOK || firstFingerprint != secondFingerprint {
		t.Fatal("normalized relation fingerprint")
	}
	foreign, _ := NewSchema(source, heapSchema)
	foreignRoot, _ := foreign.RootAt(0)
	foreignValue, _ := foreign.Of(foreignRoot, materialization.Recent)
	if schema.Admit(coordinate, foreignValue) || schema.Equal(recent, foreignValue) {
		t.Fatal("foreign family entered Escape")
	}
}

func TestEscapeRequiresExactLiveHeapLink(t *testing.T) {
	left := escapeFixture(t, "escape_live_heap_fence")
	right := escapeFixture(t, "escape_live_heap_fence")
	if left == right || left.ContentID() != right.ContentID() {
		t.Fatal("fixture must provide distinct same-content Link authorities")
	}
	leftHeap, leftHeapOK := heap.Seal(left)
	rightHeap, rightHeapOK := heap.Seal(right)
	if !leftHeapOK || !rightHeapOK || leftHeap.Link() != left || rightHeap.Link() != right || leftHeap.LinkContentID() != rightHeap.LinkContentID() {
		t.Fatal("fixture Heap authorities")
	}
	if _, ok := NewSchema(left, rightHeap); ok {
		t.Fatal("Escape accepted a same-content foreign Heap authority")
	}
	schema, ok := NewSchema(left, leftHeap)
	if !ok || !schema.Valid() || schema.Link() != left {
		t.Fatal("Escape rejected its local Heap authority")
	}
	broken := *schema.owner
	broken.heap = rightHeap
	if (Schema{owner: &broken}).Valid() {
		t.Fatal("Escape.Valid accepted a privately inconsistent Heap/Link pair")
	}
}

func TestEscapeMaterializesOnlySelectedRootAndDeduplicates(t *testing.T) {
	source := escapeFixture(t, "escape_selected_root")
	schema, _ := escapeSchema(t, source)
	if schema.RootCount() < 2 {
		t.Fatal("fixture needs two allocation roots")
	}
	first, _ := schema.RootAt(0)
	second, _ := schema.RootAt(1)
	firstRecent, _ := schema.Of(first, materialization.Recent)
	firstSummary, _ := schema.Of(first, materialization.Summary)
	secondRecent, _ := schema.Of(second, materialization.Recent)
	firstImages, _ := schema.Join(firstRecent, firstSummary)
	input, _ := schema.Join(firstImages, secondRecent)
	output, ok := schema.Materialize(input, first)
	if !ok || output.Count() != 2 {
		t.Fatal("selected materialization must collapse only the first root")
	}
	for index := 0; index < output.Count(); index++ {
		root, role, member := output.At(index)
		if !member {
			t.Fatal("normalized member missing")
		}
		if root == first && role != materialization.Summary || root == second && role != materialization.Recent {
			t.Fatal("materialization leaked across roots or retained a duplicate recent image")
		}
	}
}

func TestEscapeEnumeratesProgramRetentionBoundariesDirectly(t *testing.T) {
	source := escapeFixture(t, "escape_program_boundaries")
	schema, _ := escapeSchema(t, source)
	want := map[BoundaryKind]int{}
	mounts := source.Project().Mounts()
	for shardIndex := 0; shardIndex < mounts.Count(); shardIndex++ {
		shard, _ := mounts.At(shardIndex)
		p, _ := mounts.Program(shard)
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
			want[BoundaryCapture] += captures
		}
		for index := 0; index < writes.Count(); index++ {
			write, _ := writes.At(index)
			_, targetTerm, writeOK := writes.Get(write)
			_, _, _, _, exactLens := exactLenses.Get(targetTerm)
			_, _, _, dynamicLens := dynamicLenses.Get(targetTerm)
			if writeOK && executable.Contains(write) && (exactLens || dynamicLens) {
				want[BoundaryStore]++
			}
		}
		for index := 0; index < returns.Count(); index++ {
			term, _ := returns.At(index)
			if _, _, present := returns.Get(term); present && executable.Contains(term) {
				want[BoundaryReturn]++
			}
		}
	}
	got := map[BoundaryKind]int{}
	for index := 0; index < schema.CoordinateCount(); index++ {
		coordinate, present := schema.CoordinateAt(index)
		if !present {
			t.Fatalf("missing coordinate %d", index)
		}
		kind, present := schema.BoundaryKind(coordinate)
		if !present {
			t.Fatalf("missing coordinate kind %d", index)
		}
		got[kind]++
	}
	for _, kind := range [...]BoundaryKind{BoundaryCapture, BoundaryStore, BoundaryReturn} {
		if got[kind] != want[kind] {
			t.Fatalf("%v boundaries=%d, want %d", kind, got[kind], want[kind])
		}
	}
}
