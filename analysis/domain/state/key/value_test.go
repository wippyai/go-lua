package key

import (
	"os/exec"
	"strings"
	"testing"

	pathaddr "github.com/wippyai/go-lua/analysis/domain/path/address"
)

func TestValueConstructorsRoundTrip(t *testing.T) {
	if sym, ok := ParseSymbolValue(SymbolValue(42)); !ok || sym != 42 {
		t.Fatalf("SymbolValue/ParseSymbolValue round-trip = %d/%v, want 42/true", sym, ok)
	}
	if idx, ok := ParseReturnSlot(ReturnSlot(3)); !ok || idx != 3 {
		t.Fatalf("ReturnSlot/ParseReturnSlot round-trip = %d/%v, want 3/true", idx, ok)
	}
	if SymbolValue(0) != 0 {
		t.Fatalf("SymbolValue(0) = %d, want the zero/empty cell", uint64(SymbolValue(0)))
	}
	if ReturnSlot(-1) != 0 {
		t.Fatalf("ReturnSlot(-1) = %d, want the zero/empty cell", uint64(ReturnSlot(-1)))
	}
	if ReturnSlot(0) == 0 {
		t.Fatal("ReturnSlot(0) must be a distinct non-empty cell, not the zero sentinel")
	}
}

func TestValueKindsAreDisjoint(t *testing.T) {
	// A symbol cell and a return-slot cell with the same number must never alias.
	if SymbolValue(5) == ReturnSlot(5) {
		t.Fatal("symbol and return-slot cells with the same number collided")
	}
	if sym, ok := ParseSymbolValue(ReturnSlot(0)); ok || sym != 0 {
		t.Fatalf("ParseSymbolValue(return slot) = %d/%v, want 0/false", sym, ok)
	}
	if idx, ok := ParseReturnSlot(SymbolValue(1)); ok || idx != 0 {
		t.Fatalf("ParseReturnSlot(symbol value) = %d/%v, want 0/false", idx, ok)
	}
	if sym, ok := ParseSymbolValue(0); ok || sym != 0 {
		t.Fatalf("ParseSymbolValue(empty) = %d/%v, want 0/false", sym, ok)
	}
	if idx, ok := ParseReturnSlot(0); ok || idx != 0 {
		t.Fatalf("ParseReturnSlot(empty) = %d/%v, want 0/false", idx, ok)
	}
}

func TestAddressResolverGrammarIsOwnedElsewhere(t *testing.T) {
	sym, version, suffix, ok := pathaddr.ParseResolverPath("sym12@3.field")
	if !ok || sym != 12 || version != 3 || suffix != ".field" {
		t.Fatalf("ParseResolverPath = %d/%d/%q/%v, want 12/3/.field/true", sym, version, suffix, ok)
	}
	if got := pathaddr.VersionedRootString(42, 3); got != "sym42@3" {
		t.Fatalf("VersionedRootString = %q, want sym42@3", got)
	}
	if got := pathaddr.VersionedRootString(0, 3); got != "" {
		t.Fatalf("VersionedRootString with zero symbol = %q, want empty", got)
	}
	if got := pathaddr.VersionedRootString(42, 0); got != "" {
		t.Fatalf("VersionedRootString with zero version = %q, want empty", got)
	}
}

func TestPackageDoesNotImportIRorEngine(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", ".")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list -deps . failed: %v", err)
	}
	banned := []string{
		"github.com/wippyai/go-lua/analysis/ir",
		"github.com/wippyai/go-lua/analysis/engine",
	}
	for _, dep := range strings.Fields(string(out)) {
		for _, prefix := range banned {
			if dep == prefix || strings.HasPrefix(dep, prefix+"/") {
				t.Fatalf("key package imports forbidden dependency %q", dep)
			}
		}
	}
}
