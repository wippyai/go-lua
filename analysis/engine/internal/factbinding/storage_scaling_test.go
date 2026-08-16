package factbinding

import (
	"math/bits"
	"runtime"
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// reservedRootPages is the page span this scaling law reserves.  It is chosen
// above the largest observed single-file reservation count so the law covers
// the real workload, and low enough that a directory which grows once per page
// is still reportable instead of exhausting the machine.
const reservedRootPages = 24

// TestRootDirectoryStoresOnePageDirectoryPerCapacityStep proves the root
// directory is sized by the reserved page span, not by the number of page
// installations.  Every reservation goes through the production
// reserve/Publish pair that pendingRoot drives.
func TestRootDirectoryStoresOnePageDirectoryPerCapacityStep(t *testing.T) {
	manager, err := guard.New([]guard.Atom{1})
	if err != nil {
		t.Fatal(err)
	}
	binding, _ := newLawBinding(t, manager, false)
	store := newRootStore[planeFactor, uint64, uint64](binding.plane.domain)
	if store == nil {
		t.Fatal("root store")
	}
	empty, ok := binding.plane.domain.Empty()
	if !ok {
		t.Fatal("empty plane")
	}
	roots := reservedRootPages * rootsPerPage
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for index := 0; index < roots; index++ {
		reservation, reserved := store.reserve(empty)
		if !reserved {
			t.Fatalf("reservation %d rejected", index)
		}
		if id := reservation.Publish(); id != uint64(index)+1 {
			t.Fatalf("published id = %d, want %d", id, index+1)
		}
	}
	runtime.ReadMemStats(&after)

	directory := store.directory.Load()
	if directory == nil || directory.count.Load() != uint64(roots) {
		t.Fatalf("published root count = %d, want %d", directory.count.Load(), roots)
	}
	capacity := 1 << bits.Len(uint(reservedRootPages-1))
	if len(directory.pages) > capacity {
		t.Fatalf("directory spans %d page slots for %d pages, want at most %d", len(directory.pages), reservedRootPages, capacity)
	}
	for id := 1; id <= roots; id++ {
		if _, resolved := store.Plane(uint64(id)); !resolved {
			t.Fatalf("published root %d is unresolvable", id)
		}
	}
	// Directory bytes are bounded by twice the final capacity, page bytes by
	// the reserved span; both stay far under this ceiling, while a directory
	// that doubles on every page installation allocates orders more.
	const ceiling = 8 << 20
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > ceiling {
		t.Fatalf("reserving %d roots allocated %d bytes, want at most %d", roots, allocated, ceiling)
	}
}

// summaryScalingDepth is both the declared summary length and the number of
// independent guard atoms, so every declared key partitions into a stored and
// an absent piece and the group product is 2^depth.
const summaryScalingDepth = 10

// TestSummaryGroupScratchIsLinearInDiscoveredGroups proves the grouping
// scratch stores one cell per discovered group instead of one cell per group
// and declared key.  Discovering the full product over summaryScalingDepth
// keys creates sum(2^r) = 2*rows-2 groups across all rounds, so a scratch that
// shares prefixes stays under 3*rows cells, while a scratch that copies each
// prefix per extension stores rows*summaryScalingDepth cells.
func TestSummaryGroupScratchIsLinearInDiscoveredGroups(t *testing.T) {
	atoms := make([]guard.Atom, summaryScalingDepth)
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
	on := make([]support.Mask, summaryScalingDepth)
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

	declared := scalingDeclarations{}
	binding, ok := bindTest(scalingInput(&declared), manager)
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
	state = commit(t, work, state, candidate)
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
	patterns := make(map[uint64]int)
	rows := 0
	cells := 0
	completed := slotWork.ObserveUnder(root, declared.summary, whole, func(row carrier.ObservationRow) bool {
		observation, resolved := binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != summaryScalingDepth {
			return false
		}
		pattern := uint64(0)
		for index := 0; index < summaryScalingDepth; index++ {
			entry, ok := observation.At(index)
			if !ok {
				return false
			}
			value, present := entry.Read()
			switch {
			case present && value == uint64(index)+1:
				pattern |= 1 << uint(index)
			case !present && value == 0:
			default:
				return false
			}
		}
		patterns[pattern]++
		rows++
		cells = observationScratchCells(typed)
		return true
	})
	if !completed {
		t.Fatal("summary observation")
	}
	if !slotWork.EndObservation() {
		t.Fatal("end observation")
	}
	expected := 1 << summaryScalingDepth
	if rows != expected || len(patterns) != expected {
		t.Fatalf("summary rows = %d over %d distinct sequences, want %d of each", rows, len(patterns), expected)
	}
	for pattern, count := range patterns {
		if count != 1 {
			t.Fatalf("sequence %d observed %d times, want once", pattern, count)
		}
	}
	if ceiling := 3 * rows; cells > ceiling {
		t.Fatalf("group scratch stores %d entry cells for %d rows, want at most %d", cells, rows, ceiling)
	}
}

// observationScratchCells reports how many typed entry cells the summary
// grouping scratch retains for the discovered groups.
func observationScratchCells(work *bindingWork[uint64, uint64]) int {
	return len(work.spine)
}

type scalingDeclarations struct {
	summary carrier.Unit
	targets []carrier.Target
}

func scalingInput(declared *scalingDeclarations) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      summaryScalingDepth,
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
			units := make([]carrier.Unit, summaryScalingDepth)
			keys := make([]uint64, summaryScalingDepth)
			for key := range units {
				unit, ok := binding.DeclareExact(uint64(key))
				if !ok {
					return false
				}
				units[key], keys[key] = unit, uint64(key)
			}
			summary, ok := binding.DeclareSummary(keys)
			if !ok {
				return false
			}
			declared.summary = summary
			declared.targets = make([]carrier.Target, summaryScalingDepth)
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
