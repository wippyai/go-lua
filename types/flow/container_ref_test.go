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

func TestContainerRefStableAddressMatchesPathAddress(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(32), "rows").Field("items")
	ref, ok := ContainerRefOfPath(path)
	if !ok {
		t.Fatalf("ContainerRefOfPath(%v) failed", path)
	}
	got, ok := ref.StableAddress()
	if !ok {
		t.Fatalf("ContainerRef.StableAddress failed for %#v", ref)
	}
	want, ok := StableAddressOfPath(path)
	if !ok {
		t.Fatalf("StableAddressOfPath(%v) failed", path)
	}
	if !got.Equal(want) {
		t.Fatalf("ContainerRef.StableAddress = %#v, want %#v", got, want)
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
