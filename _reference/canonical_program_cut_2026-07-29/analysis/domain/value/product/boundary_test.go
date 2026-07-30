package product

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
)

func TestProjectBoundaryAppliesEveryDeclaredAxisPolicy(t *testing.T) {
	portableKey := axis.NewKey[synthetic]("test.boundary.portable")
	localKey := axis.NewKey[synthetic]("test.boundary.local")
	projectedKey := axis.NewKey[synthetic]("test.boundary.projected")

	portable := syntheticSpec()
	portable.Key = portableKey
	portable.Boundary = axis.PortableIdentity
	local := syntheticSpec()
	local.Key = localKey
	local.Boundary = axis.LocalOnly
	projected := syntheticSpec()
	projected.Key = projectedKey
	projected.Boundary = axis.Projected
	projected.BoundaryProject = func(value synthetic) synthetic {
		if value == syntheticLow {
			return syntheticHigh
		}
		return value
	}

	reg, err := RegistryWithAxes(portable.Erase(), local.Erase(), projected.Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes: %v", err)
	}
	value := NewWithPresence(reg, ShapeTop, presence.Present())
	value = Set(reg, value, portableKey, syntheticLow)
	value = Set(reg, value, localKey, syntheticLow)
	value = Set(reg, value, projectedKey, syntheticLow)

	got := ProjectBoundary(reg, value)
	if ShapeOf(got) != ShapeTop || !presence.Equal(PresenceOf(got), presence.Present()) {
		t.Fatalf("core boundary components changed: shape=%s presence=%s", ShapeOf(got), PresenceOf(got))
	}
	if value := Get(reg, got, portableKey); value != syntheticLow {
		t.Fatalf("portable axis = %v, want %v", value, syntheticLow)
	}
	if value := Get(reg, got, localKey); value != syntheticTop {
		t.Fatalf("local axis = %v, want top", value)
	}
	if value := Get(reg, got, projectedKey); value != syntheticHigh {
		t.Fatalf("projected axis = %v, want %v", value, syntheticHigh)
	}
	second := ProjectBoundary(reg, got)
	if !Domain(reg).Same(got, second) {
		t.Fatal("boundary projection is not representation-idempotent")
	}
}

func BenchmarkProjectBoundary(b *testing.B) {
	reg, err := RegistryWithAxes(syntheticSpec().Erase())
	if err != nil {
		b.Fatal(err)
	}
	constrained := Set(reg, Top(), syntheticKey, syntheticLow)
	b.ReportAllocs()
	b.Run("top", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ProjectBoundary(reg, Top())
		}
	})
	b.Run("portable-constrained", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			_ = ProjectBoundary(reg, constrained)
		}
	})
}
