package summary

import (
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/domain/value/axis"
	"github.com/wippyai/go-lua/analysis/domain/value/axis/presence"
	"github.com/wippyai/go-lua/analysis/domain/value/product"
)

func TestSummaryKeyComparableAndDeterministicOrdering(t *testing.T) {
	a := ref.FuncRef{Kind: ref.KindCFG, ID: 1}
	b := ref.FuncRef{Kind: ref.KindSymbol, ID: 1}
	keys := []SummaryKey{
		{Ref: b},
		{Ref: a, Entry: EntryKey{Values: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, References: 2}},
		{Ref: a},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 1}},
	}
	seen := map[SummaryKey]string{DefaultSummaryKey(a): "default"}
	if seen[SummaryKey{Ref: a}] != "default" {
		t.Fatalf("SummaryKey is not usable as expected map key")
	}

	sort.Slice(keys, func(i, j int) bool { return keys[i].Less(keys[j]) })
	want := []SummaryKey{
		{Ref: a},
		{Ref: a, Entry: EntryKey{Values: 1, References: 2}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 1}},
		{Ref: a, Entry: EntryKey{Values: 1, Facts: 2}},
		{Ref: a, Entry: EntryKey{Values: 2}},
		{Ref: b},
	}
	for i := range want {
		if keys[i] != want[i] {
			t.Fatalf("keys[%d] = %#v, want %#v", i, keys[i], want[i])
		}
	}
}

func TestSummaryKeyAxesAreDistinct(t *testing.T) {
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 1}
	values := SummaryKey{Ref: fn, Entry: EntryKey{Values: 1}}
	facts := SummaryKey{Ref: fn, Entry: EntryKey{Facts: 1}}
	references := SummaryKey{Ref: fn, Entry: EntryKey{References: 1}}

	if values == facts {
		t.Fatalf("Values and Facts keys should be distinct")
	}
	if values == references {
		t.Fatalf("Values and References keys should be distinct")
	}
	if facts == references {
		t.Fatalf("Facts and References keys should be distinct")
	}
}

func TestSnapshotExactReads(t *testing.T) {
	reg := mustRegistry(t)
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 7}
	exact := SummaryKey{Ref: fn, Entry: EntryKey{Values: 1, Facts: 2}}
	want := Summary{Returns: []product.Value{product.Top()}}
	snap := NewSnapshot(reg, EntrySummary{Key: exact, Summary: want})

	got, ok := snap.Read(exact)
	if !ok {
		t.Fatalf("Read(exact) missing")
	}
	if len(got.Returns) != 1 || !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(exact) = %#v, want one top return", got)
	}
}

func TestSnapshotExactReadsDoNotFallbackByRef(t *testing.T) {
	reg := mustRegistry(t)
	fn := ref.FuncRef{Kind: ref.KindCFG, ID: 7}
	snap := NewSnapshot(reg, EntrySummary{
		Key:     SummaryKey{Ref: fn, Entry: EntryKey{Values: 1}},
		Summary: Summary{Returns: []product.Value{product.Top()}},
	})

	if got, ok := snap.Read(DefaultSummaryKey(fn)); ok {
		t.Fatalf("Read(default same ref) = %#v, want missing exact key", got)
	}
}

func TestSnapshotReadsNormalizedSummaries(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 11})
	snap := NewSnapshot(reg, EntrySummary{
		Key: key,
		Summary: Summary{Returns: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		}},
	})

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d returns, want normalized 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Read(key) first return = %#v, want top", got.Returns[0])
	}
}

func TestSnapshotNormalizesWithCustomRegistry(t *testing.T) {
	reg, err := product.RegistryWithAxes(summaryTestSpec().Erase())
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 12})
	value := product.Set(reg, product.Top(), summaryTestKey, summaryTestLow)
	snap := NewSnapshot(reg, EntrySummary{
		Key: key,
		Summary: Summary{Returns: []product.Value{
			value,
			product.Bottom(reg),
		}},
	})

	got, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if len(got.Returns) != 1 {
		t.Fatalf("Read(key) returned %d returns, want normalized 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], value) {
		t.Fatalf("Read(key) first return was not preserved under custom registry")
	}
}

func TestSummaryCloneIsolatesReturns(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{Returns: []product.Value{product.Top(), product.Absent(reg)}}
	cloned := original.Clone()
	cloned.Returns[0] = product.Bottom(reg)

	if product.Equal(reg, original.Returns[0], product.Bottom(reg)) {
		t.Fatalf("mutating cloned returns changed original")
	}
	if !product.Equal(reg, original.Returns[0], product.Top()) {
		t.Fatalf("original first return changed unexpectedly")
	}
}

func TestSummaryCloneIsolatesNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	original := Summary{NormalReturnParams: []product.Value{product.Top(), product.Absent(reg)}}
	cloned := original.Clone()
	cloned.NormalReturnParams[0] = product.Bottom(reg)

	if product.Equal(reg, original.NormalReturnParams[0], product.Bottom(reg)) {
		t.Fatalf("mutating cloned normal return params changed original")
	}
	if !product.Equal(reg, original.NormalReturnParams[0], product.Top()) {
		t.Fatalf("original first normal return param changed unexpectedly")
	}
}

func TestSnapshotClonesOnWriteAndRead(t *testing.T) {
	reg := mustRegistry(t)
	key := DefaultSummaryKey(ref.FuncRef{Kind: ref.KindSymbol, ID: 9})
	input := Summary{Returns: []product.Value{product.Top()}}
	snap := NewSnapshot(reg, EntrySummary{Key: key, Summary: input})
	input.Returns[0] = product.Bottom(reg)

	first, ok := snap.Read(key)
	if !ok {
		t.Fatalf("Read(key) missing")
	}
	if !product.Equal(reg, first.Returns[0], product.Top()) {
		t.Fatalf("snapshot changed after input mutation")
	}

	first.Returns[0] = product.Bottom(reg)
	second, ok := snap.Read(key)
	if !ok {
		t.Fatalf("second Read(key) missing")
	}
	if !product.Equal(reg, second.Returns[0], product.Top()) {
		t.Fatalf("snapshot changed after read result mutation")
	}
}

type summaryTestAxis uint8

const (
	summaryTestBottom summaryTestAxis = iota
	summaryTestLow
	summaryTestHigh
	summaryTestTop
)

var summaryTestKey = axis.NewKey[summaryTestAxis]("test.summary.axis")

func summaryTestSpec() axis.Spec[summaryTestAxis] {
	return axis.Spec[summaryTestAxis]{
		Key:    summaryTestKey,
		Bottom: func() summaryTestAxis { return summaryTestBottom },
		Top:    func() summaryTestAxis { return summaryTestTop },
		Equal:  func(a, b summaryTestAxis) bool { return a == b },
		LessOrEq: func(a, b summaryTestAxis) bool {
			return a <= b
		},
		Join: func(a, b summaryTestAxis) summaryTestAxis {
			if a > b {
				return a
			}
			return b
		},
		Meet: func(a, b summaryTestAxis) summaryTestAxis {
			if a < b {
				return a
			}
			return b
		},
		Widen: func(prev, next summaryTestAxis) summaryTestAxis {
			if prev > next {
				return prev
			}
			return next
		},
		Hash: func(v summaryTestAxis) uint64 { return uint64(v) },
	}
}

func TestEqualTreatsAbsentReturnSlotAsBottom(t *testing.T) {
	reg := mustRegistry(t)
	empty := Summary{}
	explicitBottom := Summary{Returns: []product.Value{product.Bottom(reg)}}

	if !Equal(reg, empty, explicitBottom) {
		t.Fatalf("missing return slot should equal explicit bottom")
	}
	if !Equal(reg, explicitBottom, empty) {
		t.Fatalf("explicit bottom should equal missing return slot")
	}
}

func TestJoinWithMissingReturnSlot(t *testing.T) {
	reg := mustRegistry(t)
	got := Join(reg, Summary{}, Summary{Returns: []product.Value{product.Top()}})
	if len(got.Returns) != 1 {
		t.Fatalf("Join returned %d slots, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Join missing slot with top = %#v, want top", got.Returns[0])
	}
}

func TestNormalizeTrimsTrailingBottomReturnSlots(t *testing.T) {
	reg := mustRegistry(t)
	s := Summary{
		Returns: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		},
	}
	got := Normalize(reg, s)
	if len(got.Returns) != 1 {
		t.Fatalf("Normalize kept %d returns, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Normalize first return = %#v, want top", got.Returns[0])
	}

	allBottom := Normalize(reg, Summary{Returns: []product.Value{product.Bottom(reg)}})
	if len(allBottom.Returns) != 0 {
		t.Fatalf("Normalize(all bottom) kept %d returns, want 0", len(allBottom.Returns))
	}
}

func TestNormalizeTrimsTrailingBottomNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	s := Summary{
		NormalReturnParams: []product.Value{
			product.Top(),
			product.Bottom(reg),
			product.Bottom(reg),
		},
	}
	got := Normalize(reg, s)
	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("Normalize kept %d normal return params, want 1", len(got.NormalReturnParams))
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("Normalize first normal return param = %#v, want top", got.NormalReturnParams[0])
	}

	allBottom := Normalize(reg, Summary{NormalReturnParams: []product.Value{product.Bottom(reg)}})
	if len(allBottom.NormalReturnParams) != 0 {
		t.Fatalf("Normalize(all bottom) kept %d normal return params, want 0", len(allBottom.NormalReturnParams))
	}
}

func TestLessOrEqAndEqualForReturnTuples(t *testing.T) {
	reg := mustRegistry(t)
	bottom := Summary{}
	top := Summary{Returns: []product.Value{product.Top()}}
	topWithTrailingBottom := Summary{Returns: []product.Value{product.Top(), product.Bottom(reg)}}

	if !LessOrEq(reg, bottom, top) {
		t.Fatalf("bottom summary should be <= top-return summary")
	}
	if LessOrEq(reg, top, bottom) {
		t.Fatalf("top-return summary should not be <= bottom summary")
	}
	if !Equal(reg, top, topWithTrailingBottom) {
		t.Fatalf("trailing bottom slot should not affect equality")
	}
	if Equal(reg, bottom, top) {
		t.Fatalf("bottom summary should not equal top-return summary")
	}
}

func TestJoinWeakensNormalReturnParamConstraints(t *testing.T) {
	reg := mustRegistry(t)
	present := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	got := Join(reg,
		Summary{NormalReturnParams: []product.Value{present}},
		Summary{NormalReturnParams: []product.Value{product.Top()}},
	)

	if len(got.NormalReturnParams) != 1 {
		t.Fatalf("Join returned %d normal return params, want 1", len(got.NormalReturnParams))
	}
	if !product.Equal(reg, got.NormalReturnParams[0], product.Top()) {
		t.Fatalf("Join did not weaken normal return param to top: %v", got.NormalReturnParams[0])
	}
}

func TestLessOrEqAndEqualForNormalReturnParams(t *testing.T) {
	reg := mustRegistry(t)
	presentValue := product.NewWithPresence(reg, product.ShapeTop, presence.Present())
	bottom := Summary{}
	present := Summary{NormalReturnParams: []product.Value{presentValue}}
	presentWithTrailingBottom := Summary{NormalReturnParams: []product.Value{presentValue, product.Bottom(reg)}}

	if !LessOrEq(reg, bottom, present) {
		t.Fatalf("bottom summary should be <= present normal-return summary")
	}
	if LessOrEq(reg, present, bottom) {
		t.Fatalf("present normal-return summary should not be <= bottom summary")
	}
	if !Equal(reg, present, presentWithTrailingBottom) {
		t.Fatalf("trailing bottom normal-return slot should not affect equality")
	}
	if Equal(reg, bottom, present) {
		t.Fatalf("bottom summary should not equal present normal-return summary")
	}
}

func TestWidenWithMissingReturnSlot(t *testing.T) {
	reg := mustRegistry(t)
	got := Widen(reg, Summary{}, Summary{Returns: []product.Value{product.Top()}})
	if len(got.Returns) != 1 {
		t.Fatalf("Widen returned %d slots, want 1", len(got.Returns))
	}
	if !product.Equal(reg, got.Returns[0], product.Top()) {
		t.Fatalf("Widen missing slot with top = %#v, want top", got.Returns[0])
	}
}

func mustRegistry(t *testing.T) *axis.Registry {
	t.Helper()
	reg, err := product.RegistryWithAxes()
	if err != nil {
		t.Fatalf("RegistryWithAxes() error = %v", err)
	}
	return reg
}
