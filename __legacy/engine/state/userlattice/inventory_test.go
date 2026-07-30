package userlattice

import (
	"bytes"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
)

func TestRuntimeAndInventoryIgnoreRegistrationOrder(t *testing.T) {
	first := testInventorySpec("test.first")
	second := testInventorySpec("test.second")
	build := func(specs ...Spec) *axis.Registry {
		registry := axis.NewRegistry()
		for _, spec := range specs {
			if _, err := Register(registry, spec); err != nil {
				t.Fatal(err)
			}
		}
		return registry.Freeze()
	}
	leftRegistry := build(first, second)
	rightRegistry := build(reverseInventoryElements(second), reverseInventoryElements(first))
	leftRuntime, rightRuntime := RuntimeFor(leftRegistry), RuntimeFor(rightRegistry)
	for index := 0; index < leftRuntime.Len(); index++ {
		if leftRuntime.AxisAt(index).ID() != rightRuntime.AxisAt(index).ID() || leftRuntime.AxisAt(index).Slot() != rightRuntime.AxisAt(index).Slot() {
			t.Fatalf("runtime order differs at %d", index)
		}
	}
	left, err := Inventory(leftRegistry)
	if err != nil {
		t.Fatal(err)
	}
	right, err := Inventory(rightRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(left.CanonicalBytes(), right.CanonicalBytes()) || left.Digest() != right.Digest() {
		t.Fatal("extension inventory depends on registration order")
	}
}

func reverseInventoryElements(spec Spec) Spec {
	spec.Elements = append([]ElementID(nil), spec.Elements...)
	for left, right := 0, len(spec.Elements)-1; left < right; left, right = left+1, right-1 {
		spec.Elements[left], spec.Elements[right] = spec.Elements[right], spec.Elements[left]
	}
	return spec
}

func testInventorySpec(id AxisID) Spec {
	return Spec{
		ID: id, Elements: []ElementID{"bottom", "top"}, Bottom: "bottom", Top: "top",
		Order: []OrderPair{{Lower: "bottom", Upper: "top"}},
		Hooks: Hooks{OnAssign: AssignHook{Mode: AssignPropagate}, OnCallBoundary: CallBoundaryHook{Mode: CallBoundaryKeep}, OnClaim: []ClaimHook{{Claim: "mark", Element: "top"}}},
	}
}
