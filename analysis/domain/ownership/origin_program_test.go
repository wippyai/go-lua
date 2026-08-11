package ownership

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/heap"
	"github.com/wippyai/go-lua/program/link"
	linkproject "github.com/wippyai/go-lua/program/link/project"
	"github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

// This law deliberately reaches Program through the sealed shard topology.
// It must keep passing after Link's former retention projections are deleted.
func TestProgramRetentionOriginsUseDirectProgramCoordinates(t *testing.T) {
	p, err := lower.Lower(lower.Source{Name: "ownership_program_origins.lua", Text: []byte(`
local table = {}
local captured = 1
local closure = function() return captured end
table.value = closure
return closure
`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []linkproject.Module{{Name: "main", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	heapSchema, ok := heap.Seal(source)
	if !ok {
		t.Fatal("Heap schema")
	}
	schema, ok := NewSchema(source, heapSchema)
	if !ok {
		t.Fatal("Ownership schema")
	}
	seen := map[OriginKind]int{}
	for index := 0; index < schema.OriginCount(); index++ {
		origin, ok := schema.OriginAt(index)
		if !ok {
			t.Fatalf("OriginAt(%d)", index)
		}
		row, ok := schema.origin(origin)
		if !ok {
			t.Fatalf("origin(%d)", index)
		}
		switch row.kind {
		case OriginCaptureRetention:
			program, ok := source.Project().Mounts().Program(row.program.shard)
			if !ok || !program.Flow().Executable().Contains(row.program.term) {
				t.Fatalf("capture %d lost executable Program function", index)
			}
			_, _, ok = program.Flow().Authored().Functions().CaptureAt(row.program.term, int(row.program.index))
			if !ok {
				t.Fatalf("capture %d lost exact Program capture", index)
			}
			seen[row.kind]++
		case OriginStoreCommit:
			program, ok := source.Project().Mounts().Program(row.program.shard)
			if !ok || !program.Flow().Executable().Contains(row.program.term) {
				t.Fatalf("store %d lost executable Program write", index)
			}
			_, target, ok := program.Flow().Authored().Storage().Writes().Get(row.program.term)
			if !ok {
				t.Fatalf("store %d lost Program write", index)
			}
			_, _, _, _, lensOK := program.Flow().Authored().Access().Exact().Get(target)
			if !lensOK {
				_, _, _, lensOK = program.Flow().Authored().Access().Dynamic().Get(target)
			}
			if !lensOK {
				t.Fatalf("store %d lost Program Lens target", index)
			}
			seen[row.kind]++
		case OriginReturnBoundary:
			program, ok := source.Project().Mounts().Program(row.program.shard)
			if !ok || !program.Flow().Executable().Contains(row.program.term) {
				t.Fatalf("return %d lost executable Program return", index)
			}
			if _, _, ok := program.Flow().Authored().Control().Returns().Get(row.program.term); !ok {
				t.Fatalf("return %d lost exact Program Pack", index)
			}
			seen[row.kind]++
		}
	}
	for _, kind := range []OriginKind{OriginCaptureRetention, OriginStoreCommit, OriginReturnBoundary} {
		if seen[kind] == 0 {
			t.Fatalf("fixture omitted direct %v origin", kind)
		}
	}
}
