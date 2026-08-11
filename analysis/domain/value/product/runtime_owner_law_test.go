package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/runtimekind"
)

// The compiled product runtime is owned by the exact frozen Registry. This
// law protects the ownership cut from regressing to process-global lookup or
// from accidentally rebuilding state per query.
func TestRuntimeIsOwnedByExactFrozenRegistry(t *testing.T) {
	reg, err := RegistryWithAxes(runtimekind.Spec().Erase())
	if err != nil {
		t.Fatal(err)
	}
	first := runtimeFor(reg)
	second := runtimeFor(reg)
	if first == nil || first != second {
		t.Fatal("same frozen registry did not retain one owner-local runtime")
	}

	other, err := RegistryWithAxes(runtimekind.Spec().Erase())
	if err != nil {
		t.Fatal(err)
	}
	if first == runtimeFor(other) {
		t.Fatal("independent frozen registries shared compiled runtime state")
	}
	if first.reg != reg || runtimeFor(other).reg != other {
		t.Fatal("compiled runtime crossed its registry owner fence")
	}
}

func TestProductFreezeAtomicallyBindsExactOwner(t *testing.T) {
	reg := axis.NewRegistry()
	other := axis.NewRegistry()
	foreign := buildRegistryRuntime(other)
	if foreign.err != nil {
		t.Fatal(foreign.err)
	}
	if err := reg.FreezeWithCompiledProduct(foreign); err == nil {
		t.Fatal("mutable registry accepted a compiled product projection owned by another registry")
	}
	if reg.Frozen() {
		t.Fatal("foreign projection attempt published the registry")
	}
	owned := buildRegistryRuntime(reg)
	if owned.err != nil {
		t.Fatal(owned.err)
	}
	if err := reg.FreezeWithCompiledProduct(owned); err != nil {
		t.Fatalf("exact owner failed atomic freeze/bind: %v", err)
	}
	if runtimeFor(reg) != owned {
		t.Fatal("atomic freeze did not publish the owner-local runtime")
	}
	if err := reg.FreezeWithCompiledProduct(owned); err == nil {
		t.Fatal("frozen registry accepted a second compiled projection")
	}
}
