package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// distributiveLadder is the independent-guard ladder used by the growth law.
// Each rung declares one guard atom and one summary coordinate, so the
// correlated joint partition of the declared vector is exactly 2^N cells.
var distributiveLadder = []int{5, 10, 20, 40, 65}

type distributiveDeclarations struct {
	correlated   carrier.Unit
	distributive carrier.Unit
	targets      []carrier.Target
}

// distributiveWork is the structural cost of one observation. Every counter
// is a discrete unit of retained work, never a duration: rows are emitted
// observation records, cells are grouping spine cells, and entries are typed
// payload cells materialized into the observation slab.
type distributiveWork struct {
	rows    int
	cells   int
	entries int
}

// TestDistributiveSummaryObservationGrowsLinearlyInDeclaredCoordinates is the
// growth law for a declared coordinate-wise summary. A declared distributive
// fold must cost a + b*N structural work for N independently guarded
// coordinates: the reader folds each coordinate on its own, so the joint
// partition is never materialized.
func TestDistributiveSummaryObservationGrowsLinearlyInDeclaredCoordinates(t *testing.T) {
	for _, depth := range distributiveLadder {
		observed := observeDistributiveLadder(t, depth)
		if observed.rows != 1 {
			t.Fatalf("depth %d emitted %d rows, want 1", depth, observed.rows)
		}
		if observed.entries != depth {
			t.Fatalf("depth %d materialized %d entry cells, want %d", depth, observed.entries, depth)
		}
		// One cell per declared coordinate is the whole folded sequence. Any
		// prefix-product grouping stores at least 2^depth-1 cells.
		if observed.cells > depth {
			t.Fatalf("depth %d retained %d grouping cells, want at most %d", depth, observed.cells, depth)
		}
	}
}

// TestCorrelatedSummaryObservationRetainsJointPartition proves the fix is
// scoped to the declared fold: a summary that did not declare coordinate-wise
// distributivity still observes the exact joint partition of its coordinates.
func TestCorrelatedSummaryObservationRetainsJointPartition(t *testing.T) {
	for _, depth := range []int{5, 10} {
		binding, state, work, declared := newDistributiveBinding(t, depth)
		observed := observeUnit(t, binding, state, work, declared.correlated, depth)
		if expected := 1 << uint(depth); observed.rows != expected {
			t.Fatalf("depth %d correlated rows = %d, want %d", depth, observed.rows, expected)
		}
	}
}

// TestDistributiveSummaryEqualsCoordinatewiseFoldOfJointPartition is the
// bit-identity law. The single row a declared distributive summary emits must
// equal, cell for cell, the coordinate-wise fold a reader would compute by
// joining every row of the correlated joint partition.
func TestDistributiveSummaryEqualsCoordinatewiseFoldOfJointPartition(t *testing.T) {
	for _, depth := range []int{1, 2, 3, 4, 5} {
		binding, state, work, declared := newDistributiveBinding(t, depth)
		slotWork, ok := work.SlotWork(0)
		if !ok {
			t.Fatal("slot work")
		}
		root, ok := state.HandleAt(0)
		if !ok {
			t.Fatal("root")
		}
		whole := state.Support()

		reference := foldJointPartition(t, binding, slotWork, root, declared.correlated, whole, depth)
		folded := readDistributiveRow(t, binding, slotWork, root, declared.distributive, whole, depth)
		if len(folded) != len(reference) {
			t.Fatalf("depth %d folded %d cells, want %d", depth, len(folded), len(reference))
		}
		for index := range reference {
			value, present := folded[index].Read()
			wantValue, wantPresent := reference[index].Read()
			if present != wantPresent || present && value != wantValue {
				t.Fatalf("depth %d coordinate %d folded %d/%t, want %d/%t", depth, index, value, present, wantValue, wantPresent)
			}
		}
	}
}

// foldJointPartition is the test-only exhaustive reference. It reproduces the
// production summary consumer's fold: each coordinate is joined independently
// across every row of the correlated product, absent cells are skipped, and
// presence is the disjunction over rows.
func foldJointPartition(t testing.TB, binding *Binding[uint64, uint64], slotWork carrier.SlotWork, root carrier.RootHandle, unit carrier.Unit, within support.Mask, depth int) []ObservationEntry[uint64] {
	t.Helper()
	result := make([]ObservationEntry[uint64], depth)
	rows := 0
	if !slotWork.BeginObservation() {
		t.Fatal("begin observation")
	}
	completed := slotWork.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != depth {
			return false
		}
		rows++
		for index := 0; index < depth; index++ {
			entry, ok := observation.At(index)
			if !ok {
				return false
			}
			value, present := entry.Read()
			if !present {
				continue
			}
			if !result[index].present {
				result[index] = ObservationEntry[uint64]{value: value, present: true}
				continue
			}
			if value > result[index].value {
				result[index].value = value
			}
		}
		return true
	})
	if !completed || !slotWork.EndObservation() {
		t.Fatal("correlated observation")
	}
	if rows == 0 {
		t.Fatal("correlated observation emitted no rows")
	}
	return result
}

func readDistributiveRow(t testing.TB, binding *Binding[uint64, uint64], slotWork carrier.SlotWork, root carrier.RootHandle, unit carrier.Unit, within support.Mask, depth int) []ObservationEntry[uint64] {
	t.Helper()
	var result []ObservationEntry[uint64]
	rows := 0
	if !slotWork.BeginObservation() {
		t.Fatal("begin observation")
	}
	completed := slotWork.ObserveUnder(root, unit, within, func(row carrier.ObservationRow) bool {
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != depth {
			return false
		}
		rows++
		if !row.Region().Equal(within) {
			return false
		}
		result = make([]ObservationEntry[uint64], depth)
		for index := 0; index < depth; index++ {
			entry, ok := observation.At(index)
			if !ok {
				return false
			}
			result[index] = entry
		}
		return true
	})
	if !completed || !slotWork.EndObservation() {
		t.Fatal("distributive observation")
	}
	if rows != 1 {
		t.Fatalf("distributive observation emitted %d rows, want 1", rows)
	}
	return result
}

func observeDistributiveLadder(t testing.TB, depth int) distributiveWork {
	t.Helper()
	binding, state, work, declared := newDistributiveBinding(t, depth)
	return observeUnit(t, binding, state, work, declared.distributive, depth)
}

func observeUnit(t testing.TB, binding *Binding[uint64, uint64], state carrier.State, work *carrier.Work, unit carrier.Unit, depth int) distributiveWork {
	t.Helper()
	root, ok := state.HandleAt(0)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := work.SlotWork(0)
	if !ok {
		t.Fatal("slot work")
	}
	typed, ok := slotWork.(*bindingWork[uint64, uint64])
	if !ok {
		t.Fatal("typed slot work")
	}
	if !slotWork.BeginObservation() {
		t.Fatal("begin observation")
	}
	observed := distributiveWork{}
	completed := slotWork.ObserveUnder(root, unit, state.Support(), func(row carrier.ObservationRow) bool {
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != depth {
			return false
		}
		observed.rows++
		observed.cells = len(typed.spine)
		observed.entries = len(typed.entries)
		return true
	})
	if !completed || !slotWork.EndObservation() {
		t.Fatal("observation")
	}
	return observed
}

// newDistributiveBinding builds one plane with depth independent guard atoms
// and depth declared coordinates. Coordinate k is stored under atom k and
// absent elsewhere, so every coordinate partitions into two pieces and the
// coordinates are pairwise independent.
func newDistributiveBinding(t testing.TB, depth int) (*Binding[uint64, uint64], carrier.State, *carrier.Work, distributiveDeclarations) {
	t.Helper()
	atoms := make([]guard.Atom, depth)
	for index := range atoms {
		atoms[index] = guard.Atom(index + 1)
	}
	manager, err := guard.New(atoms)
	if err != nil {
		t.Fatal(err)
	}
	regions := support.New(manager)
	if regions == nil {
		t.Fatal("regions")
	}
	on := make([]support.Mask, depth)
	for index := range on {
		mask, ok := regions.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatal("atom literal")
		}
		on[index] = mask
	}
	whole := regions.True()
	if !regions.Seal() {
		t.Fatal("seal regions")
	}

	declared := distributiveDeclarations{}
	binding, ok := bindTest(distributiveInput(&declared, depth), manager)
	if !ok {
		t.Fatal("binding")
	}
	composition, ok := attachTestComposition(t, []carrier.FactorOperation{binding})
	if !ok {
		t.Fatal("composition")
	}
	state, ok := carrier.NewState(composition, composition.Scope(), whole)
	if !ok {
		t.Fatal("state")
	}
	work := newWork(t, composition)
	patch := binding.Begin(work, state)
	if patch == nil {
		t.Fatal("patch")
	}
	for index, target := range declared.targets {
		if !patch.Write(target, on[index], uint64(index)+1) {
			t.Fatal("staged branch write")
		}
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	return binding, commit(t, work, state, candidate), work, declared
}

func distributiveInput(declared *distributiveDeclarations, depth int) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      uint64(depth),
		Default:     0,
		AdmitAt:     func(_ uint64, _ uint64) bool { return true },
		Equal:       func(left, right uint64) bool { return left == right },
		Fingerprint: func(value uint64) uint64 { return value },
		Join: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		Widen: func(left, right uint64) uint64 {
			if left > right {
				return left
			}
			return right
		},
		LessOrEq: func(left, right uint64) bool { return left <= right },
		declare: func(binding *Binding[uint64, uint64]) bool {
			units := make([]carrier.Unit, depth)
			keys := make([]uint64, depth)
			for key := range units {
				unit, ok := binding.DeclareExact(uint64(key))
				if !ok {
					return false
				}
				units[key], keys[key] = unit, uint64(key)
			}
			correlated, ok := binding.DeclareSummary(keys)
			if !ok {
				return false
			}
			distributive, ok := binding.DeclareDistributiveSummary(keys)
			if !ok {
				return false
			}
			declared.correlated, declared.distributive = correlated, distributive
			declared.targets = make([]carrier.Target, depth)
			for index, unit := range units {
				target, accepted := binding.DeclareStrong(unit)
				if !accepted {
					return false
				}
				declared.targets[index] = target
			}
			return true
		},
	}
}
