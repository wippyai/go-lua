package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestContainerRefOfPathOwnsSymbolContainerIdentity(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(31), "rows").Field("items")

	ref, ok := ContainerRefOfPath(path)
	if !ok || !ref.IsValid() {
		t.Fatalf("ContainerRefOfPath = %#v/%v, want valid ref", ref, ok)
	}
	if got := ref.Root(); got != path.Symbol {
		t.Fatalf("ContainerRef root = %d, want %d", got, path.Symbol)
	}

	again, ok := ContainerRefOfPath(constraint.Path{Symbol: path.Symbol, Segments: path.Segments})
	if !ok || !ref.Equal(again) {
		t.Fatalf("ContainerRef equality = %#v/%#v/%v, want equal", ref, again, ok)
	}
}

func TestContainerRefRejectsUnresolvedPaths(t *testing.T) {
	if ref, ok := ContainerRefOfPath(constraint.Path{Root: "rows"}); ok || ref.IsValid() {
		t.Fatalf("ContainerRefOfPath(unresolved) = %#v/%v, want rejected", ref, ok)
	}
	if ref, ok := ContainerRefOfSymbol(0); ok || ref.IsValid() {
		t.Fatalf("ContainerRefOfSymbol(0) = %#v/%v, want rejected", ref, ok)
	}
}
