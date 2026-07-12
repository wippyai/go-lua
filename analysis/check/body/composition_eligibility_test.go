package body

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/state"
	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestCompositionEligibilityWhitelistsOnlyStraightValueWrappers(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		globals []string
		want    string
	}{
		{name: "parameter", source: `function f(x) return x end`},
		{name: "constant", source: `function f() return "ok" end`},
		{name: "stable direct call", source: `function f(x) return (g(x)) end`, globals: []string{"g"}},
		{name: "dynamic call", source: `function f(g, x) return g(x) end`, want: "call:dynamic"},
		{name: "mutation including local assignment", source: `function f(x) local y = x return y end`, want: "boundary:mutation"},
		{name: "heap read", source: `function f(x) return x.value end`, want: "boundary:heap-read"},
		{name: "allocation", source: `function f(x) return { x } end`, want: "boundary:allocation"},
		{name: "guard", source: `function f(x) if x then return x end end`, want: "shape:guard"},
		{name: "loop", source: `function f(x) while x do x = x end return x end`, want: "shape:loop"},
		{name: "vararg", source: `function f(...) return ... end`, want: "shape:vararg"},
		{name: "generic", source: `function f<T>(x: T): T return x end`, want: "shape:generic-function"},
		{name: "protected", source: `function f(x) return pcall(x) end`, globals: []string{"pcall"}, want: "call:protected"},
		{name: "operator", source: `function f(x) return x + 1 end`, want: "shape:unsupported-op"},
	}
	reg, _ := testRegistry(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fn := parseFunction(t, tc.source)
			bindings := bind.BindFunction(fn, bind.Options{Globals: tc.globals})
			prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg, Globals: tc.globals})
			if err != nil {
				t.Fatal(err)
			}
			got := prepared.CompositionEligibility()
			if got.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", got.Reason, tc.want)
			}
		})
	}
}

func TestCompositionStateCapabilitiesCoverCatalogFailClosed(t *testing.T) {
	capabilities := CompositionStateCapabilities()
	lanes := state.DefaultLanes()
	if len(capabilities) != len(lanes) || len(lanes) != 17 {
		t.Fatalf("capabilities/lanes = %d/%d, want 17/17", len(capabilities), len(lanes))
	}
	for i, capability := range capabilities {
		if capability.Lane != lanes[i] {
			t.Fatalf("capability %d lane = %q, want %q", i, capability.Lane, lanes[i])
		}
		if capability.Exact != (capability.Lane == state.LaneValues) {
			t.Fatalf("lane %q exact = %v, want value lane only", capability.Lane, capability.Exact)
		}
	}
}

func TestCompositionEligibilityReasonIsStableAcrossRepeatedPreparation(t *testing.T) {
	reg, _ := testRegistry(t)
	fn := parseFunction(t, `function f(x) while x do x = nil end return {x} end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	var want string
	for i := 0; i < 20; i++ {
		prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
		if err != nil {
			t.Fatal(err)
		}
		got := prepared.CompositionEligibility().Reason
		if i == 0 {
			want = got
		}
		if got != want {
			t.Fatalf("iteration %d reason = %q, want stable %q", i, got, want)
		}
	}
}

var benchmarkCompositionEligibility CompositionEligibility

func BenchmarkCompositionEligibilityCached(b *testing.B) {
	reg := standard.Registry()
	fn := parseFunction(b, `function f(x) return x end`)
	bindings := bind.BindFunction(fn, bind.Options{})
	prepared, err := PrepareBoundFunction(fn, bindings, Config{Registry: reg})
	if err != nil {
		b.Fatal(err)
	}
	_ = prepared.CompositionEligibility()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		benchmarkCompositionEligibility = prepared.CompositionEligibility()
	}
}
