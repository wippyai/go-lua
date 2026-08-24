package factbinding

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/engine/internal/carrier"
	"github.com/wippyai/go-lua/analysis/engine/internal/facts/support"
	"github.com/wippyai/go-lua/analysis/engine/internal/guard"
)

// constantKeyPlane is one correlated summary whose declared key vector mixes
// branched and constant coordinates. A branched coordinate is stored under its
// own guard atom and absent elsewhere, so it partitions the observed region in
// two. A constant coordinate is stored over the whole observed region, or not
// stored at all, so its partition is a single piece covering that region.
type constantKeyPlane struct {
	binding *Binding[uint64, uint64]
	state   carrier.State
	work    *carrier.Work
	summary carrier.Unit
	shape   []constantKeyCoordinate
	targets []carrier.Target
	whole   support.Mask
	manager *guard.Manager
}

// constantKeyCoordinate declares how one summary coordinate is written.
type constantKeyCoordinate struct {
	branched bool
	stored   bool
}

func branchedCoordinate() constantKeyCoordinate {
	return constantKeyCoordinate{branched: true, stored: true}
}

func constantStoredCoordinate() constantKeyCoordinate { return constantKeyCoordinate{stored: true} }

func constantAbsentCoordinate() constantKeyCoordinate { return constantKeyCoordinate{} }

// correlatedRow is one emitted observation reduced to the two things the laws
// compare: the frozen entry sequence and the region it holds over.
type correlatedRow struct {
	entries []ObservationEntry[uint64]
	region  support.Mask
}

// TestCorrelatedSummaryIssuesNoConjunctionForAConstantKey is the Boolean
// volume bound of the correlated fold. A declared key whose partition already
// covers the whole observed region distinguishes nothing inside it: every
// partial region is a subset of that region, so intersecting the two returns
// the partial unchanged. Extending the group set by such a key must therefore
// issue no region conjunction at all, however many partials the product
// carries and however many constant keys follow the first branched one.
func TestCorrelatedSummaryIssuesNoConjunctionForAConstantKey(t *testing.T) {
	for _, constant := range []int{1, 2, 4, 8} {
		shape := make([]constantKeyCoordinate, 1+constant)
		shape[0] = branchedCoordinate()
		for index := 1; index < len(shape); index++ {
			shape[index] = constantStoredCoordinate()
		}
		plane := newConstantKeyPlane(t, shape)
		rows, counters := observeCorrelatedRows(t, plane)
		if len(rows) != 2 {
			t.Fatalf("%d constant keys emitted %d rows, want 2", constant, len(rows))
		}
		if counters.SummaryExtendKeys != uint64(constant) {
			t.Fatalf("%d constant keys extended the group set %d times, want %d", constant, counters.SummaryExtendKeys, constant)
		}
		if counters.SummaryConstantKeys != uint64(constant) {
			t.Fatalf("%d constant keys were recognized %d times, want %d", constant, counters.SummaryConstantKeys, constant)
		}
		if counters.SummaryPairs != uint64(2*constant) {
			t.Fatalf("%d constant keys visited %d prefix/piece pairs, want %d", constant, counters.SummaryPairs, 2*constant)
		}
		if counters.SummaryConjunctions != 0 {
			t.Fatalf("%d constant keys issued %d region conjunctions, want 0", constant, counters.SummaryConjunctions)
		}
	}
}

// TestCorrelatedSummaryGroupsMatchExhaustiveConjunction is the exactness law
// beneath the bound. The rows a mixed branched/constant fold emits must equal,
// sequence for sequence and region for region, the exhaustive product computed
// independently from the declared writes: one row per valuation of the
// branched atoms, each carrying that valuation's cube. The rows must also
// remain an exact partition of the observed region - pairwise disjoint and
// covering - so no skipped conjunction can widen, narrow, or drop a cell.
func TestCorrelatedSummaryGroupsMatchExhaustiveConjunction(t *testing.T) {
	shape := []constantKeyCoordinate{
		branchedCoordinate(),
		constantStoredCoordinate(),
		branchedCoordinate(),
		constantAbsentCoordinate(),
		constantStoredCoordinate(),
	}
	plane := newConstantKeyPlane(t, shape)
	rows, _ := observeCorrelatedRows(t, plane)
	reference := exhaustiveCorrelatedRows(t, plane)
	if len(rows) != len(reference) {
		t.Fatalf("emitted %d rows, want %d", len(rows), len(reference))
	}

	matched := make([]bool, len(reference))
	for _, row := range rows {
		found := false
		for index, want := range reference {
			if matched[index] || !correlatedEntriesEqual(row.entries, want.entries) {
				continue
			}
			if !row.region.Equal(want.region) {
				t.Fatalf("row %v holds a region the exhaustive product does not", row.entries)
			}
			matched[index], found = true, true
			break
		}
		if !found {
			t.Fatalf("row %v is absent from the exhaustive product", row.entries)
		}
	}
	for index, ok := range matched {
		if !ok {
			t.Fatalf("exhaustive row %v was never emitted", reference[index].entries)
		}
	}

	regions := support.New(plane.manager)
	if regions == nil {
		t.Fatal("regions")
	}
	cover := regions.False()
	for outer, row := range rows {
		if !row.region.Entails(plane.whole) {
			t.Fatalf("row %v holds a region outside the observed region", row.entries)
		}
		for inner := 0; inner < outer; inner++ {
			overlap, ok := regions.And(row.region, rows[inner].region)
			if !ok {
				t.Fatal("overlap")
			}
			if !regions.Empty(overlap) {
				t.Fatalf("rows %v and %v overlap", row.entries, rows[inner].entries)
			}
		}
		joined, ok := regions.Or(cover, row.region)
		if !ok {
			t.Fatal("cover")
		}
		cover = joined
	}
	if !regions.Seal() {
		t.Fatal("seal")
	}
	if !cover.Equal(plane.whole) {
		t.Fatal("the emitted rows do not cover the observed region")
	}
}

// exhaustiveCorrelatedRows is the test-only independent reference. It reads
// nothing from the fold: it enumerates every valuation of the declared
// branched atoms, builds that valuation's cube directly from guard literals,
// and reads each coordinate's entry from the declaration that wrote it.
func exhaustiveCorrelatedRows(t testing.TB, plane constantKeyPlane) []correlatedRow {
	t.Helper()
	branched := make([]int, 0, len(plane.shape))
	for index, coordinate := range plane.shape {
		if coordinate.branched {
			branched = append(branched, index)
		}
	}
	regions := support.New(plane.manager)
	if regions == nil {
		t.Fatal("regions")
	}
	rows := make([]correlatedRow, 0, 1<<uint(len(branched)))
	for valuation := 0; valuation < 1<<uint(len(branched)); valuation++ {
		cube := regions.True()
		entries := make([]ObservationEntry[uint64], len(plane.shape))
		for index, coordinate := range plane.shape {
			if !coordinate.branched {
				entries[index] = constantKeyEntry(coordinate, index)
				continue
			}
			position := 0
			for position < len(branched) && branched[position] != index {
				position++
			}
			on := valuation&(1<<uint(position)) != 0
			literal, ok := regions.Literal(guard.Atom(index+1), on)
			if !ok {
				t.Fatal("literal")
			}
			restricted, ok := regions.And(cube, literal)
			if !ok {
				t.Fatal("cube")
			}
			cube = restricted
			if on {
				entries[index] = ObservationEntry[uint64]{value: constantKeyValue(index), present: true}
				continue
			}
			entries[index] = ObservationEntry[uint64]{}
		}
		rows = append(rows, correlatedRow{entries: entries, region: cube})
	}
	if !regions.Seal() {
		t.Fatal("seal")
	}
	return rows
}

func constantKeyEntry(coordinate constantKeyCoordinate, index int) ObservationEntry[uint64] {
	if !coordinate.stored {
		return ObservationEntry[uint64]{}
	}
	return ObservationEntry[uint64]{value: constantKeyValue(index), present: true}
}

// constantKeyValue is the stored terminal of coordinate index. It is never the
// algebra's Default, so a stored coordinate stays distinguishable from an
// absent one.
func constantKeyValue(index int) uint64 { return uint64(index) + 1 }

func correlatedEntriesEqual(left, right []ObservationEntry[uint64]) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftValue, leftPresent := left[index].Read()
		rightValue, rightPresent := right[index].Read()
		if leftPresent != rightPresent || leftPresent && leftValue != rightValue {
			return false
		}
	}
	return true
}

// observeCorrelatedRows reads the declared correlated summary once and returns
// both the emitted rows and the fold's structural counters for that single
// observation.
func observeCorrelatedRows(t testing.TB, plane constantKeyPlane) ([]correlatedRow, DbgFactBindingCounters) {
	t.Helper()
	root, ok := plane.state.HandleAt(0)
	if !ok {
		t.Fatal("root")
	}
	slotWork, ok := plane.work.SlotWork(0)
	if !ok {
		t.Fatal("slot work")
	}
	DbgFactBindingReset()
	if !slotWork.BeginObservation() {
		t.Fatal("begin observation")
	}
	var rows []correlatedRow
	completed := slotWork.ObserveUnder(root, plane.summary, plane.state.Support(), func(row carrier.ObservationRow) bool {
		observation, resolved := plane.binding.ResolveObservation(slotWork, row)
		if !resolved || observation.Count() != len(plane.shape) {
			return false
		}
		entries := make([]ObservationEntry[uint64], observation.Count())
		for index := range entries {
			entry, ok := observation.At(index)
			if !ok {
				return false
			}
			entries[index] = entry
		}
		rows = append(rows, correlatedRow{entries: entries, region: row.Region()})
		return true
	})
	counters := DbgFactBinding()
	if !completed || !slotWork.EndObservation() {
		t.Fatal("correlated observation")
	}
	return rows, counters
}

// newConstantKeyPlane declares one summary over shape and stores each
// coordinate as shape describes it. Coordinate index owns guard atom index+1;
// only a branched coordinate restricts its write to that atom.
func newConstantKeyPlane(t testing.TB, shape []constantKeyCoordinate) constantKeyPlane {
	t.Helper()
	atoms := make([]guard.Atom, len(shape))
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
	whole := regions.True()
	writes := make([]support.Mask, len(shape))
	for index, coordinate := range shape {
		if !coordinate.branched {
			writes[index] = whole
			continue
		}
		literal, ok := regions.Literal(guard.Atom(index+1), true)
		if !ok {
			t.Fatal("atom literal")
		}
		writes[index] = literal
	}
	if !regions.Seal() {
		t.Fatal("seal regions")
	}

	plane := constantKeyPlane{shape: shape, whole: whole, manager: manager}
	binding, ok := bindTest(constantKeyInput(&plane, shape), manager)
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
	for index, coordinate := range shape {
		if !coordinate.stored {
			continue
		}
		if !patch.Write(plane.targets[index], writes[index], constantKeyValue(index)) {
			t.Fatal("staged write")
		}
	}
	candidate, ok := patch.Accept(work)
	if !ok {
		t.Fatal("accept")
	}
	plane.binding, plane.work = binding, work
	plane.state = commit(t, work, state, candidate)
	if !plane.state.Support().SameHandle(whole) {
		t.Fatal("the observed region is not the region the constant coordinates were written over")
	}
	return plane
}

func constantKeyInput(plane *constantKeyPlane, shape []constantKeyCoordinate) testAlgebraInput[uint64, uint64] {
	return testAlgebraInput[uint64, uint64]{
		KeyEnd:      uint64(len(shape)),
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
			keys := make([]uint64, len(shape))
			units := make([]carrier.Unit, len(shape))
			for index := range shape {
				unit, ok := binding.DeclareExact(uint64(index))
				if !ok {
					return false
				}
				keys[index], units[index] = uint64(index), unit
			}
			summary, ok := binding.DeclareSummary(keys)
			if !ok {
				return false
			}
			targets := make([]carrier.Target, len(shape))
			for index, unit := range units {
				target, accepted := binding.DeclareStrong(unit)
				if !accepted {
					return false
				}
				targets[index] = target
			}
			plane.summary, plane.targets = summary, targets
			return true
		},
	}
}
