package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
)

func TestOverlayFunctionRefsIgnoresLegacyLivePathKey(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(70), "fn").Field("call")
	addr := testStableAddressPath(t, path)
	baseRef := FunctionRef{GraphID: 1}
	legacyRef := FunctionRef{GraphID: 2}
	base := WithFunctionRefAddress(nil, addr, FunctionRefSetOf(baseRef))
	live := FunctionRefs{
		path.Key(): FunctionRefSetOf(legacyRef),
	}

	got := OverlayFunctionRefs(base, live)
	set, ok := FunctionRefAtAddress(got, addr)
	if !ok {
		t.Fatalf("overlay dropped base ref: %#v", got)
	}
	ref, singleton := set.Singleton()
	if !singleton || ref != baseRef {
		t.Fatalf("legacy live key overrode base ref: %s", set.Format())
	}
}

func TestOverlayClosureRefsIgnoresLegacyLivePathKey(t *testing.T) {
	path := constraint.NewPath(cfg.SymbolID(71), "closure").Field("call")
	addr := testStableAddressPath(t, path)
	baseRef := ClosureRefOf(FunctionRef{GraphID: 3}, CaptureCellsDomain.Bottom(), nil)
	legacyRef := ClosureRefOf(FunctionRef{GraphID: 4}, CaptureCellsDomain.Bottom(), nil)
	base := WithClosureRefAddress(nil, addr, ClosureRefSetOf(baseRef))
	live := ClosureRefs{
		path.Key(): ClosureRefSetOf(legacyRef),
	}

	got := OverlayClosureRefs(base, live)
	set, ok := ClosureRefAtAddress(got, addr)
	if !ok {
		t.Fatalf("overlay dropped base ref: %#v", got)
	}
	ref, singleton := set.Singleton()
	if !singleton || ref.Ref != baseRef.Ref {
		t.Fatalf("legacy live key overrode base closure ref: %s", set.Format())
	}
}
