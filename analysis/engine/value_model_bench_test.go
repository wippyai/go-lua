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
//   - BenchmarkRetainedHeapSlope measures retained bytes at N and 2N members of
//     the same shape over one fixed Factor population. It is the decisive gate:
//     the row cut is justified by the marginal slope, not by the ns/op of a
//     dispatch. It reports the slope, the itemized census of one member's
//     cluster, and the factor table's flat absolute total.

package engine

import (
	"context"
	"runtime"
	"testing"
	"unsafe"

	"github.com/wippyai/go-lua/analysis/engine/internal/equation"
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
			key, keyed := fixture.queries[0].PublicationKey()
			if !keyed {
				b.Fatal("wide member fan query has no snapshot key")
			}
			value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
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
	// benchDeclaredFactors is the declared Factor population of a program. Each
	// domain owner declares exactly one Factor slot unconditionally, so the count
	// is bounded by the owner set: a 4061-member program declares the same five
	// slots a 15-member program declares. The factor table is flat and is
	// reported as one absolute total, never as a per-member share.
	benchDeclaredFactors = 5
	// benchRetainedHeapMembers is N. Its double stays inside the fixture's
	// semantic identity budget, so both populations are built by the same
	// generator with the same per-member shape.
	benchRetainedHeapMembers = 24
	// benchRetainedHeapReplicas raises the retained population above the noise
	// floor of a post-collection HeapAlloc reading.
	benchRetainedHeapReplicas = 16
	// benchTypedAccessClosures is the closure count newTypedOutputAccess installs
	// per member. Each field is a separate heap funcval whose captured set has no
	// struct width, so the census counts them and the marginal slope prices them.
	benchTypedAccessClosures = 7
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
// A member here is one Rule/Query pair over an already declared Factor, and the
// rows it produces. The declared Factor population is held at
// benchDeclaredFactors while the member count doubles, which is the ratio a
// program has: a member joins an existing domain owner instead of declaring a
// slot of its own.
//
// Three figures come out of one measurement and answer three different
// questions. B/marginal-member is the decisive slope. B/member-cluster is the
// itemized census of the objects one member's cluster is made of, so the row cut
// names which objects it removes. B/factor-table is the flat program-wide total
// of the factor table, which the slope must never carry.
//
// Run it with -benchtime=1x. Each iteration builds and retains two whole
// populations, so repeating it measures the same slope at higher cost.
func BenchmarkRetainedHeapSlope(b *testing.B) {
	for index := 0; index < b.N; index++ {
		single, singleObjects := benchRetainedSolvedPopulation(b, benchRetainedHeapMembers)
		double, doubleObjects := benchRetainedSolvedPopulation(b, 2*benchRetainedHeapMembers)
		if single <= 0 || double <= 0 {
			b.Fatalf("retained heap measurement is not positive: N=%d bytes 2N=%d bytes", single, double)
		}
		if double <= single {
			b.Fatalf("doubling the members did not raise retained heap: N=%d bytes 2N=%d bytes", single, double)
		}
		members := float64(benchRetainedHeapReplicas * benchRetainedHeapMembers)
		marginal := float64(double-single) / members
		census, items := benchMemberClusterCensus()
		b.ReportMetric(marginal, "B/marginal-member")
		b.ReportMetric(float64(census), "B/member-cluster")
		b.ReportMetric(float64(benchFactorTableBytes()), "B/factor-table")
		for _, item := range items {
			b.Logf("member cluster: %-34s %d x %4d B = %5d B %s", item.name, item.count, item.width, item.count*item.width, item.note)
		}
		b.Logf("member cluster: %d B of statically sized objects against a %.0f B marginal member, plus one execution closure, one rebinding closure and %d typed access closures per member",
			census, marginal, benchTypedAccessClosures)
		b.Logf("member cluster: %.1f marginal live heap objects per member", float64(doubleObjects-singleObjects)/members)
	}
}

// benchMemberClusterItem is one object of a member's retained cluster, at the
// count one member holds and the width the type has.
type benchMemberClusterItem struct {
	name     string
	count    uintptr
	width    uintptr
	retained bool
	note     string
}

// benchMemberClusterCensus itemizes the statically sized objects one solved
// member is made of and returns their per-member total. Closures are counted
// rather than sized: a funcval's captured set has no struct width, so the total
// is the floor the marginal slope is read against.
//
// The draft wrapper is itemized and excluded from the total: the sealed row mints
// its execution closure over the bound rule, and the compilation releases the
// drafts, so the wrapper is the object the row cut removes rather than an object
// a solved member holds.
func benchMemberClusterCensus() (uintptr, []benchMemberClusterItem) {
	items := []benchMemberClusterItem{
		{name: "boundRule", count: 1, width: unsafe.Sizeof(boundRule[uint64, ruleUnit]{}), retained: true, note: "held by the row's execution closure"},
		{name: "outputRuntime", count: 1, width: unsafe.Sizeof(outputRuntime{}), retained: true, note: "one staged-target projection per member"},
		{name: "outputWriteRuntime", count: 1, width: unsafe.Sizeof(outputWriteRuntime{}), retained: true, note: "the projection's one-element write vector"},
		{name: "memberRow", count: 1, width: unsafe.Sizeof(memberRow{}), retained: true, note: "the member's row in the sealed program"},
		{name: "boundRuleMember", count: 1, width: unsafe.Sizeof(boundRuleMember[uint64, ruleUnit]{}), note: "draft, released at seal"},
	}
	var total uintptr
	for _, item := range items {
		if !item.retained {
			continue
		}
		total += item.count * item.width
	}
	return total, items
}

// benchFactorTableBytes is the sealed factor table's whole width: one record and
// one typed owner reference per declared Factor. It is a program constant, so it
// is reported absolutely.
func benchFactorTableBytes() uintptr {
	return benchDeclaredFactors * (unsafe.Sizeof(factorRecord{}) + unsafe.Sizeof(runtimeFactor(nil)))
}

// benchRetainedSolvedPopulation builds benchRetainedHeapReplicas solvers of
// count members each over benchDeclaredFactors Factors, solves and retains every
// one of them with its published State, and reports the heap and the object
// count those live objects hold after a collection.
func benchRetainedSolvedPopulation(b *testing.B, count int) (int64, int64) {
	b.Helper()
	// The timer runs through construction on purpose. This benchmark reports its
	// verdict through custom metrics, and an untimed body would report a
	// near-zero ns/op that a time-based -benchtime would answer by raising b.N
	// until it built the populations thousands of times over.
	solvers := make([]*Solver, 0, benchRetainedHeapReplicas)
	states := make([]*State, 0, benchRetainedHeapReplicas)
	queries := make([][]ProgramQuery, 0, benchRetainedHeapReplicas)

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	for replica := 0; replica < benchRetainedHeapReplicas; replica++ {
		fixture := newBenchFixedFactorPopulation(b, count)
		state, status := fixture.solver.Solve(context.Background())
		if state == nil || status != SolveComplete {
			b.Fatalf("retained population solve at %d members = state:%t status:%v", count, state != nil, status)
		}
		// Every member is read back. Members of one Factor separate by coordinate,
		// so a population whose members collapsed onto one cell would answer a
		// joined value here instead of its own.
		for index, query := range fixture.queries {
			key, keyed := query.PublicationKey()
			if !keyed {
				b.Fatalf("retained population query[%d] has no snapshot key", index)
			}
			value, readable := testSnapshotQueryValue[uint64](fixture.solver, state, key)
			if !readable || value != fixture.expected[index] {
				b.Fatalf("retained population query[%d] at %d members = %d/%t, want %d/true", index, count, value, readable, fixture.expected[index])
			}
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
	liveObjects := int64(after.Mallocs-after.Frees) - int64(before.Mallocs-before.Frees)
	return int64(after.HeapAlloc) - int64(before.HeapAlloc), liveObjects
}

// benchFixedFactorPopulation is a solved population whose declared Factor count
// is benchDeclaredFactors at every member count.
type benchFixedFactorPopulation struct {
	solver   *Solver
	queries  []ProgramQuery
	expected []uint64
}

// newBenchFixedFactorPopulation builds members Rule/Query pairs over
// benchDeclaredFactors Factor slots: member i writes and reads factor
// i%benchDeclaredFactors at its own Point, so members of one Factor separate by
// coordinate the way a program's members do.
//
// It is not newReceiptQueryMatrixFixture. That fixture declares one Factor per
// member, which is the shape a permutation law needs and the one shape a
// per-member slope cannot be read from: doubling its count doubles the declared
// Factor population with it.
func newBenchFixedFactorPopulation(b *testing.B, members int) benchFixedFactorPopulation {
	b.Helper()
	if members < benchDeclaredFactors || 1+3*members > 256 {
		b.Fatalf("fixed factor population of %d members is outside the semantic identity budget", members)
	}
	builder := NewSchema()
	factors := make([]*FactorSlot[uint64], benchDeclaredFactors)
	reads := make([]SchemaReadForm[uint64], benchDeclaredFactors)
	writes := make([]SchemaWriteForm[uint64], benchDeclaredFactors)
	for owner := range factors {
		factor, factorOK := DeclareFactorSlot[uint64](builder, coldKey(960_000+owner))
		write, writeOK := factor.ExactWrite()
		read, readOK := factor.ExactRead()
		if !factorOK || !writeOK || !readOK {
			b.Fatal("fixed factor population Factor declaration")
		}
		factors[owner], reads[owner], writes[owner] = factor, read, write
	}
	rules := make([]*RuleSlot[uint64, ruleUnit], members)
	writeSlots := make([]SchemaWriteSlot[uint64], members)
	queries := make([]*QuerySlot[uint64], members)
	for index := 0; index < members; index++ {
		owner := index % benchDeclaredFactors
		rule, ruleOK := DeclareRuleSlot[uint64, ruleUnit](builder, SchemaRuleSpec[uint64]{
			Semantic: coldKey(961_000 + index), OperandFamily: unitOperandFamily, Inputs: 0,
			Admission: SchemaAdmission{Basis: RuleAdmissionBasisTrustedTheorem, Identity: coldKey(962_000 + index)}, Output: factors[owner].Ref(),
		})
		writeOK := false
		if ruleOK {
			writeSlots[index], writeOK = SchemaWrite(rule, writes[owner])
		}
		if !ruleOK || !writeOK {
			b.Fatal("fixed factor population Rule declaration")
		}
		rules[index] = rule
	}
	for index := 0; index < members; index++ {
		query, queryOK := DeclareQuerySlot[uint64](builder, SchemaQuerySpec{Semantic: coldKey(963_000 + index), Freezer: coldKey(964_000 + index)})
		if !queryOK || !SchemaQueryRead(query, reads[index%benchDeclaredFactors]) {
			b.Fatal("fixed factor population Query declaration")
		}
		queries[index] = query
	}
	schema, schemaOK := builder.Seal()
	if !schemaOK || schema == nil {
		b.Fatal("fixed factor population schema seal")
	}

	binding := NewSchemaBinding(schema)
	if binding == nil {
		b.Fatal("fixed factor population binding")
	}
	for owner := range factors {
		if !BindFactor(binding, factors[owner], hotUintFactorSpec()) {
			b.Fatal("fixed factor population Factor binding")
		}
	}
	for index := 0; index < members; index++ {
		owner := index % benchDeclaredFactors
		value := uint64(index + 1)
		ruleSpec := HotRuleSpec[uint64, ruleUnit]{
			OperandContent: ruleUnitContent,
			Admission:      AdmitRuleByTrustedTheorem[uint64, ruleUnit](coldKey(962_000 + index)),
			Transfer: func(access Access[uint64, ruleUnit]) bool {
				return Product(access, func(row Row) bool { return StageValue(access, row, value) })
			},
		}
		if !BindRule[uint64, uint64, ruleUnit](binding, rules[index], writeSlots[index], factors[owner], ruleSpec, testRuleProjector[ruleUnit]) {
			b.Fatal("fixed factor population Rule binding")
		}
	}
	for index := 0; index < members; index++ {
		spec := hotExactQuerySpec()
		spec.Result.Semantic = coldKey(964_000 + index)
		spec.Project = func(cells OrderedCells[uint64]) uint64 {
			value, present, valid := cells.At(0)
			if !valid || !present {
				return 0
			}
			return value
		}
		if !BindExactQuery(binding, queries[index], factors[index%benchDeclaredFactors], spec) {
			b.Fatal("fixed factor population Query binding")
		}
	}
	if !binding.Seal() {
		b.Fatal("fixed factor population binding seal")
	}

	ruleImplementations := make([]*RuleImplementation[uint64, uint64, ruleUnit], members)
	queryImplementations := make([]*ExactQueryImplementation[uint64, uint64], members)
	for index := 0; index < members; index++ {
		var ruleOK, queryOK bool
		ruleImplementations[index], ruleOK = RuleImplementationAt[uint64, uint64, ruleUnit](binding, rules[index])
		queryImplementations[index], queryOK = ExactQueryImplementationAt[uint64, uint64](binding, queries[index])
		if !ruleOK || ruleImplementations[index] == nil || !queryOK || queryImplementations[index] == nil {
			b.Fatal("fixed factor population implementation receipt")
		}
	}

	assembly, assemblyOK := binding.beginBindingTopologyBuilder()
	if !assemblyOK || assembly == nil {
		b.Fatal("fixed factor population assembly")
	}
	sites := make([]equation.Site, members)
	occurrences := make([]equation.Occurrence, members)
	operands := make([]equation.Operand, members)
	operandValues := make([]ruleUnit, members)
	for index := 0; index < members; index++ {
		site, siteOK := assembly.admitSite(compositionKeyOf(coldKey(965_000+index)), equation.EmptyScope(), equation.TrueExpr(), equation.InitPresent)
		occurrence, occurrenceOK := assembly.admitAt(site)
		value := ruleUnitForSemantic(coldKey(966_000 + index))
		entity, entityOK := operandEntityForContent(value.content)
		operand, operandOK := assembly.admitOperand(occurrence, entity)
		if !siteOK || !occurrenceOK || !entityOK || !operandOK {
			b.Fatal("fixed factor population source admission")
		}
		sites[index], occurrences[index], operands[index], operandValues[index] = site, occurrence, operand, value
	}
	if !assembly.SealSources() {
		b.Fatal("fixed factor population source seal")
	}
	declaration := topologyDeclaration{binding: binding, batch: assembly.inner.batch}
	for index := 0; index < members; index++ {
		declaration.points = append(declaration.points, declaredPointRow{ID: receiptAssemblySemanticID(byte(1 + index)), Site: sites[index]})
	}
	for index := 0; index < members; index++ {
		proof := ruleImplementations[index].binding.proof
		source, sourceOK := assembly.issueRuleSurfaceSource(equation.RuleSurfaceSourceSpec{Schema: proof.semantic, OperandFamily: proof.operandFamily, Occurrence: occurrences[index], Operand: operands[index], Writes: []equation.ResolvedWrite{{Index: 0, Surface: equation.Surface{Factor: proof.output, Form: equation.SurfaceWriteExact, Local: 1, Mode: equation.TargetModeStrong}}}})
		draft, draftOK := ruleImplementations[index].beginBindingRuleRow(source)
		part, partOK := ruleImplementations[index].WritePart(source, 0)
		rowOK := sourceOK && draftOK && partOK && draft.AddWrite(part)
		row, issued := assembly.issueRuleRow(draft)
		if !rowOK || !issued {
			b.Fatal("fixed factor population Rule topology")
		}
		declaration.members = append(declaration.members, declaredMemberRow{Plane: declaredMemberOwner, ID: receiptAssemblySemanticID(byte(1 + members + index)), Row: row.row})
	}
	for index := 0; index < members; index++ {
		queryOrdinal, queryOrdinalOK := queries[index].Ordinal()
		factorOrdinal, factorOrdinalOK := factors[index%benchDeclaredFactors].Ordinal()
		if !queryOrdinalOK || !factorOrdinalOK {
			b.Fatal("fixed factor population ordinal mapping")
		}
		instance := equation.QueryInstance{Family: schema.querySemanticAt(queryOrdinal), Point: equation.PointAt(index), Surfaces: []equation.Surface{{Factor: schema.factorSemanticAt(factorOrdinal), Form: equation.SurfaceReadExact, Local: 1}}}
		declaration.queries = append(declaration.queries, declaredQueryRow{ID: receiptAssemblySemanticID(byte(1 + 2*members + index)), Row: instance})
	}
	constructed, refusal := constructTopology(declaration)
	topology, issued, committed := constructed.topology, constructed.graph, !refusal.Available() && constructed.Available()
	graph := CommittedProgramFrom(topology, issued)
	if !committed || graph == nil {
		b.Fatalf("fixed factor population graph commit stage=%v step=%v ordinal=%d", refusal.Stage(), refusal.Step(), refusal.Ordinal())
	}
	compilation, compilationOK := BeginProgramConstruction(binding, graph)
	if !compilationOK || compilation == nil {
		b.Fatal("fixed factor population compilation")
	}
	for index := 0; index < members; index++ {
		if _, memberOK := graph.RuleMember(receiptAssemblySemanticID(byte(1 + members + index))); !memberOK {
			b.Fatal("fixed factor population Rule member receipt")
		}
		if !installConstOperandResolver(ruleImplementations[index], operandValues[index]) {
			b.Fatal("fixed factor population resolver")
		}
		if attached := AttachRuleMember(compilation, ruleImplementations[index], receiptAssemblySemanticID(byte(1+members+index))); !attached {
			b.Fatal("fixed factor population Rule attachment")
		}
	}
	queryReceipts := make([]ProgramQuery, members)
	for index := 0; index < members; index++ {
		query, queryOK := graph.Query(receiptAssemblySemanticID(byte(1 + 2*members + index)))
		if !queryOK || !AttachExactQuery(compilation, queryImplementations[index], receiptAssemblySemanticID(byte(1+2*members+index))) {
			b.Fatal("fixed factor population Query attachment")
		}
		queryReceipts[index] = query
	}
	solver, _, solverOK := compilation.Seal()
	if !solverOK || solver == nil {
		b.Fatal("fixed factor population Solver")
	}
	expected := make([]uint64, members)
	for index := range expected {
		expected[index] = uint64(index + 1)
	}
	return benchFixedFactorPopulation{solver: solver, queries: queryReceipts, expected: expected}
}
