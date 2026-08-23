package snapshot

import (
	"errors"
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

var (
	axisDeltaSchema = identity.ContentID{0xA1, 0xD1}
	axisDeltaStore  = identity.StoreID(0xA1)
	axisDeltaAxis   = Axis[int, int]{SchemaID: axisDeltaSchema, Slot: 0}
	axisDeltaColumn = identity.ContentID{0xA1, 0xC1}
	axisDeltaSink   int
)

func axisDeltaLessInt(left, right int) bool { return left < right }

func axisDeltaJoinMax(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func axisDeltaBase(t testing.TB, rows map[int]int) Snapshot {
	t.Helper()
	builder := NewBuilder(axisDeltaSchema, axisDeltaStore, identity.Generation(1))
	if err := PutColumn(&builder, axisDeltaAxis, Content[int, int]{Rows: rows}); err != nil {
		t.Fatalf("put axis delta column: %v", err)
	}
	if err := builder.Publish(axisDeltaColumn, axisDeltaAxis.Slot); err != nil {
		t.Fatalf("publish axis delta column: %v", err)
	}
	sealed, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal axis delta base: %v", err)
	}
	return sealed
}

func TestAxisDeltaAppendFlushesCanonicalJoinedRows(t *testing.T) {
	base := axisDeltaBase(t, map[int]int{3: 30})
	delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 4, axisDeltaLessInt)
	if !delta.Available() {
		t.Fatal("axis delta is unavailable")
	}
	for _, row := range []struct{ key, value int }{
		{key: 5, value: 5},
		{key: 1, value: 1},
		{key: 3, value: 35},
		{key: 5, value: 9},
	} {
		if err := delta.Append(row.key, row.value); err != nil {
			t.Fatalf("append (%d,%d): %v", row.key, row.value, err)
		}
	}
	if delta.Len() != 4 || delta.Cap() != 4 {
		t.Fatalf("delta shape = (%d,%d), want (4,4)", delta.Len(), delta.Cap())
	}

	builder := NewDelta(base, identity.Generation(2))
	if err := delta.Flush(&builder); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if delta.Len() != 0 || delta.Cap() != 4 {
		t.Fatalf("post-flush shape = (%d,%d), want (0,4)", delta.Len(), delta.Cap())
	}
	published, err := builder.Seal()
	if err != nil {
		t.Fatalf("seal flushed builder: %v", err)
	}
	for _, want := range []struct{ key, value int }{{1, 1}, {3, 35}, {5, 9}} {
		value, status := Read(&published, axisDeltaAxis, want.key)
		if value != want.value || status != ReadHit {
			t.Fatalf("published key %d = (%d,%v), want (%d,hit)", want.key, value, status, want.value)
		}
	}
	if value, status := Read(&base, axisDeltaAxis, 5); status != ReadMiss || value != 0 {
		t.Fatalf("base key 5 = (%d,%v), want (0,miss)", value, status)
	}
}

func TestAxisDeltaDuplicateJoinIsInputOrderIndependent(t *testing.T) {
	base := axisDeltaBase(t, nil)
	first := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 3, axisDeltaLessInt)
	second := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 3, axisDeltaLessInt)
	for _, row := range []struct{ key, value int }{{7, 2}, {7, 9}, {7, 4}} {
		if err := first.Append(row.key, row.value); err != nil {
			t.Fatalf("first append: %v", err)
		}
	}
	for _, row := range []struct{ key, value int }{{7, 4}, {7, 2}, {7, 9}} {
		if err := second.Append(row.key, row.value); err != nil {
			t.Fatalf("second append: %v", err)
		}
	}

	firstBuilder := NewDelta(base, identity.Generation(2))
	secondBuilder := NewDelta(base, identity.Generation(3))
	if err := first.Flush(&firstBuilder); err != nil {
		t.Fatalf("first flush: %v", err)
	}
	if err := second.Flush(&secondBuilder); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	firstSnapshot, err := firstBuilder.Seal()
	if err != nil {
		t.Fatalf("first seal: %v", err)
	}
	secondSnapshot, err := secondBuilder.Seal()
	if err != nil {
		t.Fatalf("second seal: %v", err)
	}
	firstValue, firstStatus := Read(&firstSnapshot, axisDeltaAxis, 7)
	secondValue, secondStatus := Read(&secondSnapshot, axisDeltaAxis, 7)
	if firstValue != 9 || secondValue != 9 || firstStatus != ReadHit || secondStatus != ReadHit {
		t.Fatalf("joined values = (%d,%v), (%d,%v), want (9,hit), (9,hit)", firstValue, firstStatus, secondValue, secondStatus)
	}
}

func TestAxisDeltaInvalidInputsAndCapacity(t *testing.T) {
	cases := []struct {
		name string
		axis Axis[int, int]
		join AxisDeltaJoin[int]
		cap  int
		less AxisDeltaLess[int]
	}{
		{name: "unavailable axis", axis: Axis[int, int]{}, join: axisDeltaJoinMax, cap: 1, less: axisDeltaLessInt},
		{name: "nil join", axis: axisDeltaAxis, cap: 1, less: axisDeltaLessInt},
		{name: "negative capacity", axis: axisDeltaAxis, join: axisDeltaJoinMax, cap: -1, less: axisDeltaLessInt},
		{name: "nil less", axis: axisDeltaAxis, join: axisDeltaJoinMax, cap: 1},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			delta := NewAxisDelta(testCase.axis, testCase.join, testCase.cap, testCase.less)
			if delta.Available() {
				t.Fatal("invalid construction was admitted")
			}
			if err := delta.Append(1, 1); !errors.Is(err, ErrAxisDeltaInvalid) {
				t.Fatalf("append error = %v, want %v", err, ErrAxisDeltaInvalid)
			}
		})
	}

	delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 1, axisDeltaLessInt)
	if err := delta.Append(1, 10); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := delta.Append(2, 20); !errors.Is(err, ErrAxisDeltaFull) {
		t.Fatalf("capacity error = %v, want %v", err, ErrAxisDeltaFull)
	}
	if delta.Len() != 1 {
		t.Fatalf("failed append changed len to %d, want 1", delta.Len())
	}
}

func TestAxisDeltaFailedFlushPreservesRows(t *testing.T) {
	delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 4, axisDeltaLessInt)
	for _, row := range []struct{ key, value int }{{5, 5}, {1, 1}, {5, 9}} {
		if err := delta.Append(row.key, row.value); err != nil {
			t.Fatalf("append: %v", err)
		}
	}
	builder := NewBuilder(axisDeltaSchema, axisDeltaStore, identity.Generation(1))
	if err := delta.Flush(&builder); !errors.Is(err, ErrUnknownSlot) {
		t.Fatalf("failed flush = %v, want unknown slot", err)
	}
	if delta.Len() != 3 {
		t.Fatalf("failed flush len = %d, want 3", delta.Len())
	}
	// Flush may canonicalize its staging order, but every staged row remains.
	seen := map[int]int{}
	for index := 0; index < delta.Len(); index++ {
		key, value, ok := delta.At(index)
		if !ok {
			t.Fatalf("staged row %d unavailable", index)
		}
		seen[key] = value
	}
	if len(seen) != 2 || seen[1] != 1 || seen[5] != 9 {
		t.Fatalf("preserved rows = %#v, want keys 1 and 5 with values 1 and 9", seen)
	}
}

func TestAxisDeltaResetRetainsCapacityAndAllocatesNothing(t *testing.T) {
	for _, width := range []int{0, 1, 8, 64, 512} {
		width := width
		delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, width, axisDeltaLessInt)
		if allocations := testing.AllocsPerRun(100, func() {
			for key := 0; key < width; key++ {
				if err := delta.Append(key, key); err != nil {
					t.Fatalf("width %d append: %v", width, err)
				}
			}
			axisDeltaSink = delta.Len()
			delta.Reset()
		}); allocations != 0 {
			t.Fatalf("width %d append/reset allocations = %v, want 0", width, allocations)
		}
		if delta.Len() != 0 || delta.Cap() != width {
			t.Fatalf("width %d reset shape = (%d,%d), want (0,%d)", width, delta.Len(), delta.Cap(), width)
		}
	}
}

func TestAxisDeltaLessTiePreservesRows(t *testing.T) {
	equalAll := func(left, right int) bool { return left == right }
	delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, 2, equalAll)
	if err := delta.Append(1, 10); err != nil {
		t.Fatalf("append first: %v", err)
	}
	if err := delta.Append(2, 20); err != nil {
		t.Fatalf("append second: %v", err)
	}
	builder := NewDelta(axisDeltaBase(t, nil), identity.Generation(2))
	if err := delta.Flush(&builder); !errors.Is(err, ErrAxisDeltaOrder) {
		t.Fatalf("less tie = %v, want %v", err, ErrAxisDeltaOrder)
	}
	if delta.Len() != 2 {
		t.Fatalf("less tie dropped rows; len = %d, want 2", delta.Len())
	}
}

func BenchmarkAxisDeltaAppendResetWidth512(b *testing.B) {
	const width = 512
	delta := NewAxisDelta(axisDeltaAxis, axisDeltaJoinMax, width, axisDeltaLessInt)
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		for key := 0; key < width; key++ {
			if err := delta.Append(key, key); err != nil {
				b.Fatal(err)
			}
		}
		axisDeltaSink = delta.Len()
		delta.Reset()
	}
}
