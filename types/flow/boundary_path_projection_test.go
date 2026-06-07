package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestBoundaryPathProjectionProjectsParamAndReturnRoots(t *testing.T) {
	sym := cfg.SymbolID(42)
	path := constraint.NewPath(sym, "record").Field("node").Field("id")
	projection := NewBoundaryPathProjection(
		map[cfg.SymbolID]int{sym: 1},
		map[cfg.SymbolID][]BoundaryPath{
			sym: {{
				Kind:     BoundaryPathReturn,
				Index:    0,
				Segments: constraint.NewPath(sym, "record").Field("node").Segments,
			}},
		},
	)

	addr, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatalf("stable address for path %s", path.Key())
	}
	paths := projection.PathsFromAddress(addr)
	if len(paths) != 2 {
		t.Fatalf("projected paths = %v, want param and return", paths)
	}
	if paths[0].Kind != BoundaryPathParam || paths[0].Index != 1 || len(paths[0].Segments) != 2 {
		t.Fatalf("param path = %#v", paths[0])
	}
	if paths[1].Kind != BoundaryPathReturn || paths[1].Index != 0 || len(paths[1].Segments) != 1 || paths[1].Segments[0].Name != "id" {
		t.Fatalf("return path = %#v", paths[1])
	}
}

func TestBoundaryParamPathFromKeyProjectsSourceSubtree(t *testing.T) {
	sym := cfg.SymbolID(7)
	source := constraint.NewPath(sym, "payload").Field("items")
	target := source.IndexStr("primary").Field("value")

	path, ok := BoundaryParamPathFromPath(target, source, 2)
	if !ok {
		t.Fatal("expected target below source to project")
	}
	if path.Kind != BoundaryPathParam || path.Index != 2 {
		t.Fatalf("boundary path root = %#v", path)
	}
	if len(path.Segments) != 2 || path.Segments[0].Name != "primary" || path.Segments[1].Name != "value" {
		t.Fatalf("boundary suffix = %#v", path.Segments)
	}

	sibling := constraint.NewPath(sym, "payload").Field("other")
	if _, ok := BoundaryParamPathFromPath(sibling, source, 2); ok {
		t.Fatal("sibling path should not project")
	}
}
