package refinement

import (
	"testing"

	"github.com/wippyai/go-lua/__legacy/analysis/test/value/standard"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/analysis/type/typeexpr"
)

func TestPartitionTruthyOptionalNonBooleanHasExactNilComplement(t *testing.T) {
	reg := standard.Registry()
	optionalString := typeexpr.Optional(typ.String)
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, optionalString), optionalString)

	falsy, exact := PartitionTruthiness(reg, value, false)
	if !exact {
		t.Fatal("optional string falsy partition is not exact")
	}
	if got := product.PresenceOf(falsy); !presence.Equal(got, presence.Absent()) {
		t.Fatalf("optional string falsy presence = %s, want absent", got)
	}
	got, ok := typevalue.StructuralTypeOf(reg, nil, falsy, typevalue.StructuralTypeOptions{ApplyPresence: true})
	if !ok || !typ.TypeEquals(got, typ.Nil) {
		t.Fatalf("optional string falsy type = %v/%v, want nil", got, ok)
	}
}

func TestPartitionTruthinessPreservesBooleanWitness(t *testing.T) {
	reg := standard.Registry()
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)

	truthy, exact := PartitionTruthiness(reg, value, true)
	if !exact {
		t.Fatal("boolean truthy partition is not exact")
	}
	got, ok := typevalue.TypeOf(reg, truthy)
	if !ok || !typ.TypeEquals(got, typ.True) {
		t.Fatalf("truthy boolean type = %v/%v, want true", got, ok)
	}

	falsy, exact := PartitionTruthiness(reg, value, false)
	if !exact {
		t.Fatal("boolean falsy partition is not exact")
	}
	got, ok = typevalue.TypeOf(reg, falsy)
	if !ok || !typ.TypeEquals(got, typ.False) {
		t.Fatalf("falsy boolean type = %v/%v, want false", got, ok)
	}
}

type truthinessTestAxis uint8

const (
	truthinessTestBottom truthinessTestAxis = iota
	truthinessTestMarked
	truthinessTestTop
)

var truthinessTestKey = axis.NewKey[truthinessTestAxis]("test.refinement.truthiness_preserved")

func TestPartitionTruthinessPreservesIndependentAxis(t *testing.T) {
	spec := axis.Spec[truthinessTestAxis]{
		Key:      truthinessTestKey,
		Bottom:   func() truthinessTestAxis { return truthinessTestBottom },
		Top:      func() truthinessTestAxis { return truthinessTestTop },
		Equal:    func(a, b truthinessTestAxis) bool { return a == b },
		LessOrEq: func(a, b truthinessTestAxis) bool { return a <= b },
		Join: func(a, b truthinessTestAxis) truthinessTestAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b truthinessTestAxis) truthinessTestAxis {
			if a < b {
				return a
			}
			return b
		},
		Hash:      func(value truthinessTestAxis) uint64 { return uint64(value) },
		Boundary:  axis.PortableIdentity,
		Retention: axis.ImmutableRetention[truthinessTestAxis](),
		Canonical: axis.PendingCanonical[truthinessTestAxis]("test-only axis"),
	}
	reg, err := standard.RegistryWithAxes(spec.Erase())
	if err != nil {
		t.Fatal(err)
	}
	value := typevalue.WithWitness(reg, typevalue.FromType(reg, typ.Boolean), typ.Boolean)
	value = product.Set(reg, value, truthinessTestKey, truthinessTestMarked)

	for _, wantTruthy := range []bool{false, true} {
		part, exact := PartitionTruthiness(reg, value, wantTruthy)
		if !exact {
			t.Fatalf("partition(%v) is not exact", wantTruthy)
		}
		if got := product.Get(reg, part, truthinessTestKey); got != truthinessTestMarked {
			t.Fatalf("partition(%v) independent axis = %v, want marked", wantTruthy, got)
		}
	}
}

func TestPartitionTruthinessDoesNotClaimUnknownComplement(t *testing.T) {
	reg := standard.Registry()
	value := product.Top()
	part, exact := PartitionTruthiness(reg, value, true)
	if exact {
		t.Fatal("top truthy complement was claimed exact")
	}
	if !product.Equal(reg, part, value) {
		t.Fatal("unrepresentable truthy complement changed value")
	}
}
