package factbinding

import (
	"strconv"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/carrier/shape"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// TestCarrierMatrixExactKeyIndependence is a typed semantic law, deliberately
// separate from the resource matrix. Every populated key
// is written through its own opaque target and observed through its own opaque
// exact unit; neither carrier nor the test recovers a Binding-private key.
func TestCarrierMatrixExactKeyIndependence(t *testing.T) {
	for _, factors := range []int{3, 9, 16, 25} {
		for _, keys := range []int{1, 16, 128} {
			t.Run("factors="+strconv.Itoa(factors)+"/populated-keys="+strconv.Itoa(keys), func(t *testing.T) {
				manager, err := guard.New(nil)
				if err != nil {
					t.Fatal(err)
				}
				whole, ok := support.True(manager)
				if !ok {
					t.Fatal("whole support")
				}
				operations := make([]carrier.FactorOperation, factors)
				bindings := make([]*Binding[uint64, uint64], factors)
				declared := make([]matrixExactDeclarations, factors)
				for index := range operations {
					binding, capabilities, valid := newMatrixExactBinding(manager, keys)
					if !valid {
						t.Fatal("binding")
					}
					operations[index], bindings[index], declared[index] = binding, binding, capabilities
				}
				composition, ok := attachTestComposition(t, operations)
				if !ok || composition.Count() != factors {
					t.Fatal("composition")
				}
				state, ok := carrier.NewState(composition, composition.Scope(), whole)
				if !ok {
					t.Fatal("state")
				}
				work := newWork(t, composition)
				anchor := factors / 2
				patch := bindings[anchor].Begin(work, state)
				if patch == nil {
					t.Fatal("stage exact-key writes")
				}
				for key, target := range declared[anchor].targets {
					if !patch.Write(target, whole, uint64(key+1)) {
						t.Fatalf("stage key %d", key)
					}
				}
				accepted, ok := patch.Accept(work)
				if !ok {
					t.Fatal("accept exact-key writes")
				}
				next := commit(t, work, state, accepted)
				root, ok := next.HandleAt(shape.Slot(anchor))
				if !ok {
					t.Fatal("anchor root")
				}
				for key, unit := range declared[anchor].units {
					got, present, valid := observedExactValue(bindings[anchor], work, root, unit, whole, func(guard.Atom) bool { return false })
					if !valid || !present || got != uint64(key+1) {
						t.Fatalf("key %d = %d/%t/%t, want %d/true/true", key, got, present, valid, key+1)
					}
				}
			})
		}
	}
}

type matrixExactDeclarations struct {
	units   []carrier.Unit
	targets []carrier.Target
}

func newMatrixExactBinding(manager *guard.Manager, keys int) (*Binding[uint64, uint64], matrixExactDeclarations, bool) {
	var declared matrixExactDeclarations
	config := testAlgebraInput[uint64, uint64]{
		KeyEnd:      uint64(keys),
		Default:     0,
		AdmitAt:     func(uint64, uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join:        matrixExactMax,
		Widen:       matrixExactMax,
		LessOrEq:    func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			declared.units = make([]carrier.Unit, keys)
			declared.targets = make([]carrier.Target, keys)
			for index := range declared.units {
				unit, ok := binding.DeclareExact(uint64(index))
				if !ok {
					return false
				}
				declared.units[index] = unit
			}
			for index, unit := range declared.units {
				target, ok := binding.DeclareStrong(unit)
				if !ok {
					return false
				}
				declared.targets[index] = target
			}
			return true
		},
	}
	binding, ok := bindTest(config, manager)
	return binding, declared, ok
}

func matrixExactMax(left, right uint64) uint64 {
	if left > right {
		return left
	}
	return right
}

func BenchmarkCarrierMatrixExactKeys(b *testing.B) {
	for _, factors := range []int{3, 9, 16, 25} {
		for _, keys := range []int{1, 16, 128} {
			b.Run("factors="+strconv.Itoa(factors)+"/populated-keys="+strconv.Itoa(keys), func(b *testing.B) {
				manager, err := guard.New(nil)
				if err != nil {
					b.Fatal(err)
				}
				whole, ok := support.True(manager)
				if !ok {
					b.Fatal("whole support")
				}
				operations := make([]carrier.FactorOperation, factors)
				bindings := make([]*Binding[uint64, uint64], factors)
				declarations := make([]matrixExactDeclarations, factors)
				for slot := range operations {
					binding, declared, valid := newMatrixExactBinding(manager, keys)
					if !valid {
						b.Fatal("binding")
					}
					operations[slot], bindings[slot], declarations[slot] = binding, binding, declared
				}
				composition, ok := attachTestComposition(b, operations)
				if !ok {
					b.Fatal("composition")
				}
				state, ok := carrier.NewState(composition, composition.Scope(), whole)
				if !ok {
					b.Fatal("state")
				}
				work := newWork(b, composition)
				anchor := factors / 2
				binding, declared := bindings[anchor], declarations[anchor]
				b.ReportAllocs()
				b.ResetTimer()
				for index := 0; index < b.N; index++ {
					patch := binding.Begin(work, state)
					if patch == nil {
						b.Fatal("begin")
					}
					for key, target := range declared.targets {
						if !patch.Write(target, whole, uint64(key+1)) {
							b.Fatal("write")
						}
					}
					if !patch.Discard() {
						b.Fatal("discard")
					}
				}
				b.ReportMetric(float64(factors), "factor-slots/op")
				b.ReportMetric(float64(keys), "exact-key-writes/op")
			})
		}
	}
}
