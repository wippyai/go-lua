// value_model_bench_test.go is the measurement lane for the value-model round.
// Three benchmarks feed three separate decisions and nothing else:
//
//   - BenchmarkDispatchMatrix measures the layout matrix in isolation, on
//     synthetic types that exist only in this file. It answers how a per-row
//     judgment should be reached and where the row should live: one-method
//     interface against stored function, narrow row against wide row, value-row
//     walk against pointer-row walk, small result against a result wide enough
//     to be returned through memory, and a homogeneous row sequence against a
//     heterogeneous one. It deliberately touches no production type: a matrix
//     measured through today's row would measure today's row.
//
//   - BenchmarkSolveFixpoint measures real solves through the engine's only
//     supported construction route, so the row cut has a BEFORE lane on the
//     shapes it will change: a wide member fan and a deep recurrence. Both
//     shapes come from the fixtures the engine laws already build; the widths
//     and epoch drive are the only additions.
//
//   - BenchmarkRetainedHeapSlope measures retained bytes per member at N and 2N
//     members of the same shape. It is the decisive gate: the row cut is
//     justified by the marginal slope, not by the ns/op of a dispatch.

package engine

import (
	"context"
	"runtime"
	"testing"
	"unsafe"
)

// benchWideResult is a result too large to travel in registers, so a
// measurement over it includes the copy every dispatch form pays.
type benchWideResult struct {
	words [34]uint64
}

const benchWideResultBytes = 272

// benchTransfer is the one-method interface arm of the matrix.
type benchTransfer interface {
	apply(seed uint64) uint64
}

// benchWideTransfer is the same arm returning a result of benchWideResultBytes.
type benchWideTransfer interface {
	applyWide(seed uint64) benchWideResult
}

type benchTransferSum struct{ bias uint64 }

func (transfer benchTransferSum) apply(seed uint64) uint64 { return seed + transfer.bias }

func (transfer benchTransferSum) applyWide(seed uint64) benchWideResult {
	var result benchWideResult
	result.words[0] = seed + transfer.bias
	result.words[len(result.words)-1] = seed ^ transfer.bias
	return result
}

type benchTransferMix struct{ bias uint64 }

func (transfer benchTransferMix) apply(seed uint64) uint64 {
	return seed ^ (transfer.bias * 0x9E3779B97F4A7C15)
}

func (transfer benchTransferMix) applyWide(seed uint64) benchWideResult {
	var result benchWideResult
	result.words[0] = seed ^ transfer.bias
	result.words[len(result.words)-1] = seed + transfer.bias
	return result
}

func benchApplySum(seed, bias uint64) uint64 { return seed + bias }

func benchApplyMix(seed, bias uint64) uint64 { return seed ^ (bias * 0x9E3779B97F4A7C15) }

func benchApplyWideSum(seed, bias uint64) benchWideResult {
	var result benchWideResult
	result.words[0] = seed + bias
	result.words[len(result.words)-1] = seed ^ bias
	return result
}

// benchInterfaceRow is the interface-carrying row. An interface value is two
// words, so this row is the same width as the narrow function-field row and the
// two are directly comparable.
type benchInterfaceRow struct {
	transfer benchTransfer
	seed     uint64
}

type benchWideInterfaceRow struct {
	transfer benchWideTransfer
	seed     uint64
}

// benchNarrowRow is the narrow function-field row: one stored function and the
// operands it is called with.
type benchNarrowRow struct {
	apply func(seed, bias uint64) uint64
	seed  uint64
	bias  uint64
}

// benchWideRow carries the same function field inside a row wide enough that a
// sequential walk pays a separate cache line per row.
type benchWideRow struct {
	apply   func(seed, bias uint64) uint64
	seed    uint64
	bias    uint64
	payload [8]uint64
}

type benchWideResultRow struct {
	apply func(seed, bias uint64) benchWideResult
	seed  uint64
	bias  uint64
}

const (
	benchNarrowRowBytes = 24
	benchWideRowMinimum = 80
)

// benchDispatchRows is the row count every arm walks. It is sized so the whole
// population of the widest arm stays well inside a modern last-level cache,
// which keeps the measurement about dispatch and row width rather than about
// main-memory bandwidth.
const benchDispatchRows = 1024

// benchSink absorbs every dispatched result so no arm can be folded away.
var benchSink uint64

// TestDispatchMatrixRowsHaveTheModelledLayout keeps the matrix honest. Each arm
// claims a row width in its name, and a silently repacked row would turn the
// narrow-against-wide comparison into a comparison of two identical walks.
func TestDispatchMatrixRowsHaveTheModelledLayout(t *testing.T) {
	if size := unsafe.Sizeof(benchNarrowRow{}); size != benchNarrowRowBytes {
		t.Errorf("narrow function-field row is %d bytes, the matrix models %d", size, benchNarrowRowBytes)
	}
	if size := unsafe.Sizeof(benchInterfaceRow{}); size != benchNarrowRowBytes {
		t.Errorf("interface row is %d bytes, the matrix compares it against a %d-byte row", size, benchNarrowRowBytes)
	}
	if size := unsafe.Sizeof(benchWideResultRow{}); size != benchNarrowRowBytes {
		t.Errorf("wide-result row is %d bytes, the matrix models %d", size, benchNarrowRowBytes)
	}
	if size := unsafe.Sizeof(benchWideRow{}); size < benchWideRowMinimum {
		t.Errorf("wide row is %d bytes, the matrix models at least %d", size, benchWideRowMinimum)
	}
	if size := unsafe.Sizeof(benchWideResult{}); size != benchWideResultBytes {
		t.Errorf("wide result is %d bytes, the matrix models %d", size, benchWideResultBytes)
	}
}

// BenchmarkDispatchMatrix feeds the row-representation decision: which dispatch
// form and which row placement the cut adopts. Every arm performs exactly one
// dispatch per iteration over a sequential row walk, so ns/op is directly
// comparable across arms and allocs/op proves no arm boxes per call.
func BenchmarkDispatchMatrix(b *testing.B) {
	interfaceRows := make([]benchInterfaceRow, benchDispatchRows)
	heterogeneousInterfaceRows := make([]benchInterfaceRow, benchDispatchRows)
	wideInterfaceRows := make([]benchWideInterfaceRow, benchDispatchRows)
	narrowRows := make([]benchNarrowRow, benchDispatchRows)
	heterogeneousNarrowRows := make([]benchNarrowRow, benchDispatchRows)
	wideRows := make([]benchWideRow, benchDispatchRows)
	heterogeneousWideRows := make([]benchWideRow, benchDispatchRows)
	wideResultRows := make([]benchWideResultRow, benchDispatchRows)
	for index := range narrowRows {
		seed, bias := uint64(index), uint64(index*7+1)
		interfaceRows[index] = benchInterfaceRow{transfer: benchTransferSum{bias: bias}, seed: seed}
		wideInterfaceRows[index] = benchWideInterfaceRow{transfer: benchTransferSum{bias: bias}, seed: seed}
		narrowRows[index] = benchNarrowRow{apply: benchApplySum, seed: seed, bias: bias}
		wideRows[index] = benchWideRow{apply: benchApplySum, seed: seed, bias: bias}
		wideResultRows[index] = benchWideResultRow{apply: benchApplyWideSum, seed: seed, bias: bias}
		if index%2 == 0 {
			heterogeneousInterfaceRows[index] = benchInterfaceRow{transfer: benchTransferSum{bias: bias}, seed: seed}
			heterogeneousNarrowRows[index] = benchNarrowRow{apply: benchApplySum, seed: seed, bias: bias}
			heterogeneousWideRows[index] = benchWideRow{apply: benchApplySum, seed: seed, bias: bias}
			continue
		}
		heterogeneousInterfaceRows[index] = benchInterfaceRow{transfer: benchTransferMix{bias: bias}, seed: seed}
		heterogeneousNarrowRows[index] = benchNarrowRow{apply: benchApplyMix, seed: seed, bias: bias}
		heterogeneousWideRows[index] = benchWideRow{apply: benchApplyMix, seed: seed, bias: bias}
	}

	// A pointer row is measured twice: over a contiguous backing array, which
	// isolates the extra indirection, and over individually allocated rows,
	// which is what a pointer row actually costs once a program has run for a
	// while and its rows no longer sit next to each other.
	contiguousPointerRows := make([]*benchNarrowRow, benchDispatchRows)
	for index := range narrowRows {
		contiguousPointerRows[index] = &narrowRows[index]
	}
	scatteredPointerRows := make([]*benchNarrowRow, benchDispatchRows)
	scatterFiller := make([][]uint64, benchDispatchRows)
	for index := range scatteredPointerRows {
		row := new(benchNarrowRow)
		*row = narrowRows[index]
		scatteredPointerRows[index] = row
		// Separate consecutive rows so the walk cannot inherit the locality of
		// one allocation run.
		scatterFiller[index] = make([]uint64, 8)
	}
	runtime.KeepAlive(scatterFiller)

	mask := uint64(benchDispatchRows - 1)
	arms := []struct {
		name string
		walk func(b *testing.B)
	}{
		{"direct-call/row24/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &narrowRows[uint64(index)&mask]
				sink += benchApplySum(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"interface/row24/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &interfaceRows[uint64(index)&mask]
				sink += row.transfer.apply(row.seed)
			}
			benchSink += sink
		}},
		{"interface/row24/small/heterogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &heterogeneousInterfaceRows[uint64(index)&mask]
				sink += row.transfer.apply(row.seed)
			}
			benchSink += sink
		}},
		{"interface/row24/wide-result/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &wideInterfaceRows[uint64(index)&mask]
				result := row.transfer.applyWide(row.seed)
				sink += result.words[0] + result.words[len(result.words)-1]
			}
			benchSink += sink
		}},
		{"funcfield/row24/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &narrowRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"funcfield/row24/small/heterogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &heterogeneousNarrowRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"funcfield/row88/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &wideRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"funcfield/row88/small/heterogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &heterogeneousWideRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"funcfield/row24/wide-result/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := &wideResultRows[uint64(index)&mask]
				result := row.apply(row.seed, row.bias)
				sink += result.words[0] + result.words[len(result.words)-1]
			}
			benchSink += sink
		}},
		{"funcfield/pointer-row-contiguous/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := contiguousPointerRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
		{"funcfield/pointer-row-scattered/small/homogeneous", func(b *testing.B) {
			var sink uint64
			for index := 0; index < b.N; index++ {
				row := scatteredPointerRows[uint64(index)&mask]
				sink += row.apply(row.seed, row.bias)
			}
			benchSink += sink
		}},
	}
	for _, arm := range arms {
		b.Run(arm.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			arm.walk(b)
		})
	}
}

// benchSolveFixpointWidth is the wide arm's member count. It stays inside the
// receipt-matrix fixture's semantic identity budget while giving the executor a
// fan wide enough that per-member cost dominates fixed solve overhead.
const benchSolveFixpointWidth = 25

// benchIdentityOrder is the fixture's declaration and row order.
func benchIdentityOrder(count int) []int {
	order := make([]int, count)
	for index := range order {
		order[index] = index
	}
	return order
}

// BenchmarkSolveFixpoint feeds the row-cut cost decision: what a real solve
// costs today, per shape, before any representation changes. It drives the
// engine's only supported construction route, so the numbers describe the
// production spine rather than a bench-local imitation of it.
//
// The wide arm is a fan of one-member groups, which is the widest member
// population the supported route builds: every fixture in this package issues
// one group per Rule row, so a multi-member group fold has no BEFORE lane here
// until a fixture produces one.
//
// Construction is excluded from the measurement on the two cold arms. The
// reconverge arm excludes construction by not repeating it: it revises both
// external producers and drives the executor's recurrence back to a fixpoint,
// which is the shape a working analysis spends its time in.
func BenchmarkSolveFixpoint(b *testing.B) {
	order := benchIdentityOrder(benchSolveFixpointWidth)

	b.Run("wide-member-fan/cold", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			fixture := newReceiptQueryMatrixFixture(b, benchSolveFixpointWidth, order, order)
			b.StartTimer()
			state, status := fixture.solver.Solve(context.Background())
			if state == nil || status != SolveComplete {
				b.Fatalf("wide member fan solve = state:%t status:%v", state != nil, status)
			}
			value, readable := ReceiptQueryResult[uint64](fixture.queries[0], fixture.solver, state)
			if !readable || value != fixture.expected[0] {
				b.Fatalf("wide member fan query = %d/%t, want %d/true", value, readable, fixture.expected[0])
			}
		}
	})

	b.Run("deep-recurrence/cold", func(b *testing.B) {
		b.ReportAllocs()
		for index := 0; index < b.N; index++ {
			b.StopTimer()
			fixture := newDiagnosticsExternalInterfaceFixture(b)
			b.StartTimer()
			state, status := fixture.solver.Solve(context.Background())
			if state == nil || status != SolveComplete {
				b.Fatalf("deep recurrence solve = state:%t status:%v", state != nil, status)
			}
		}
	})

	b.Run("deep-recurrence/reconverge", func(b *testing.B) {
		fixture := newDiagnosticsExternalInterfaceFixture(b)
		epoch, epochOK := newRuntimeEpoch(fixture.solver.runtime, fixture.solver.relation, context.Background())
		if !epochOK || epoch == nil {
			b.Fatal("deep recurrence epoch")
		}
		defer epoch.discard()
		if !epoch.run() {
			b.Fatal("deep recurrence initial epoch")
		}
		b.ReportAllocs()
		b.ResetTimer()
		for index := 0; index < b.N; index++ {
			for _, group := range fixture.sourceGroups {
				if !epoch.markDirty(group) {
					b.Fatal("deep recurrence source revision")
				}
			}
			if !epoch.run() {
				b.Fatal("deep recurrence revision epoch")
			}
		}
	})
}

const (
	// benchRetainedHeapMembers is N. Its double stays inside the receipt-matrix
	// fixture's semantic identity budget, so both populations are built by the
	// same generator with the same per-member shape.
	benchRetainedHeapMembers = 24
	// benchRetainedHeapReplicas raises the retained population above the noise
	// floor of a post-collection HeapAlloc reading.
	benchRetainedHeapReplicas = 16
)

// BenchmarkRetainedHeapSlope feeds the decisive question of the row cut: what
// each additional solved member costs in retained memory. A dispatch that is
// nanoseconds faster does not justify the cut; a marginal slope that falls
// does.
//
// The slope is marginal on purpose. Absolute retained bytes at one size include
// every fixed cost of a sealed schema, binding and topology, which does not
// scale with members and would flatter or damn the representation by accident.
// Measuring N and 2N of the same shape cancels that fixed part: the difference
// is what the members themselves retain.
//
// A member here is one Factor/Rule/Query triple and the rows it produces,
// because that is the unit the receipt-matrix fixture scales: doubling count
// doubles the declared slots, the topology rows and the solved cells together.
// The reported figure is therefore per triple, not per Rule row alone.
//
// Run it with -benchtime=1x. Each iteration builds and retains two whole
// populations, so repeating it measures the same slope at higher cost.
func BenchmarkRetainedHeapSlope(b *testing.B) {
	for index := 0; index < b.N; index++ {
		single := benchRetainedSolvedPopulation(b, benchRetainedHeapMembers)
		double := benchRetainedSolvedPopulation(b, 2*benchRetainedHeapMembers)
		if single <= 0 || double <= 0 {
			b.Fatalf("retained heap measurement is not positive: N=%d bytes 2N=%d bytes", single, double)
		}
		if double <= single {
			b.Fatalf("doubling the members did not raise retained heap: N=%d bytes 2N=%d bytes", single, double)
		}
		members := float64(benchRetainedHeapReplicas * benchRetainedHeapMembers)
		b.ReportMetric(float64(double-single)/members, "B/marginal-member")
		b.ReportMetric(float64(single)/members, "B/member-at-N")
		b.ReportMetric(float64(double)/(2*members), "B/member-at-2N")
	}
}

// benchRetainedSolvedPopulation builds benchRetainedHeapReplicas solvers of
// count members each, solves and retains every one of them with its published
// State, and reports the heap those live objects hold after a collection.
func benchRetainedSolvedPopulation(b *testing.B, count int) int64 {
	b.Helper()
	// The timer runs through construction on purpose. This benchmark reports its
	// verdict through custom metrics, and an untimed body would report a
	// near-zero ns/op that a time-based -benchtime would answer by raising b.N
	// until it built the populations thousands of times over.
	order := benchIdentityOrder(count)
	solvers := make([]*Solver, 0, benchRetainedHeapReplicas)
	states := make([]*State, 0, benchRetainedHeapReplicas)
	queries := make([][]ReceiptQuery, 0, benchRetainedHeapReplicas)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	for replica := 0; replica < benchRetainedHeapReplicas; replica++ {
		fixture := newReceiptQueryMatrixFixture(b, count, order, order)
		state, status := fixture.solver.Solve(context.Background())
		if state == nil || status != SolveComplete {
			b.Fatalf("retained population solve at %d members = state:%t status:%v", count, state != nil, status)
		}
		solvers = append(solvers, fixture.solver)
		states = append(states, state)
		queries = append(queries, fixture.queries)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)

	// Every solver, State and query receipt is still reachable at the reading
	// above, which is what makes the reading a measurement of retention rather
	// than of allocation.
	runtime.KeepAlive(solvers)
	runtime.KeepAlive(states)
	runtime.KeepAlive(queries)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc)
}
