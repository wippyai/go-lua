package carrier_test

import (
	"testing"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/lattice"
	latticelaws "github.com/wippyai/go-lua/analysis/test/laws/lattice"
	"github.com/wippyai/go-lua/analysis/type/typ"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestCarrierLawsAndExactMu(t *testing.T) {
	table, integer, text, universe := fixture(t)
	bottom, ok := carrier.Bottom(table, universe)
	if !ok {
		t.Fatal("carrier bottom")
	}
	first := mustCarrier(t, table, universe, integer, origin.New(originAt(t, universe, 0)))
	second := mustCarrier(t, table, universe, text, origin.New(originAt(t, universe, 1), originAt(t, universe, 2)))
	top := mustTop(t, table, universe)

	latticelaws.LawSuite[carrier.Value]{
		Name: "type-factor-carrier",
		Domain: lattice.Lattice[carrier.Value]{
			Bottom:   func() carrier.Value { return bottom },
			Top:      func() carrier.Value { return top },
			Equal:    carrier.Equal,
			LessOrEq: carrier.LessEqual,
			Join: func(left, right carrier.Value) carrier.Value {
				return mustJoin(t, left, right)
			},
			Widen: func(previous, next carrier.Value) carrier.Value {
				return mustWiden(t, previous, next)
			},
		},
		Sample: []carrier.Value{bottom, first, second, top},
	}.Run(t)

	joined := mustJoin(t, first, second)
	mu := mustMu(t, first, second)
	if !carrier.Equal(mu, joined) {
		t.Fatal("Mu closure diverged from exact carrier join")
	}
	origins, ok := joined.Origins()
	if !ok || origins.Count() != 3 {
		t.Fatalf("joined origins=%v / %v, want three exact witnesses", origins.Count(), ok)
	}
	if _, ok := joined.Data(); !ok {
		t.Fatal("finite joined carrier lost Pack")
	}
}

func TestCarrierNarrowIsExplicitlyUnavailable(t *testing.T) {
	table, integer, _, universe := fixture(t)
	finite := mustCarrier(t, table, universe, integer, origin.New(originAt(t, universe, 0)))
	if _, ok := carrier.Narrow(mustTop(t, table, universe), finite); ok {
		t.Fatal("carrier fabricated a narrowing operation for Pack")
	}
	if _, ok := carrier.Narrow(finite, finite); ok {
		t.Fatal("carrier advertised finite Pack narrowing")
	}
}

func TestCarrierRejectsInfeasibleOrForeignFiniteProducts(t *testing.T) {
	table, integer, _, universe := fixture(t)
	if _, ok := carrier.New(table, universe, table.Bottom(), origin.New(originAt(t, universe, 0))); ok {
		t.Fatal("origin-bearing uninhabited carrier admitted")
	}
	if _, ok := carrier.New(table, universe, typedomain.Pack{}, origin.Empty()); ok {
		t.Fatal("zero Pack admitted")
	}

	other, err := typedomain.NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	otherLabel, err := other.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	other.Seal()
	otherPack, ok := other.Closed(otherLabel)
	if !ok {
		t.Fatal("other Pack")
	}
	left := mustCarrier(t, table, universe, integer, origin.Empty())
	right, ok := carrier.New(other, universe, otherPack, origin.Empty())
	if !ok {
		t.Fatal("other carrier")
	}
	if _, ok := carrier.Join(left, right); ok {
		t.Fatal("foreign Type Tables joined through carrier")
	}
}

func TestFactorTopHasNoFiniteProjection(t *testing.T) {
	table, _, _, universe := fixture(t)
	top := mustTop(t, table, universe)
	if data, ok := top.Data(); ok || !data.IsBottom() {
		t.Fatal("FactorTop exposed a Pack")
	}
	if origins, ok := top.Origins(); ok || origins.Count() != 0 {
		t.Fatal("FactorTop exposed finite origins")
	}
	if top.Hash() == 0 {
		t.Fatal("FactorTop has no stable fingerprint")
	}
}

func TestCarrierRetainsSourceMajorOriginGroupingWithoutAllocation(t *testing.T) {
	table, integer, _, universe := fixture(t)
	value := mustCarrier(t, table, universe, integer, origin.New(
		originAt(t, universe, 3), originAt(t, universe, 2),
		originAt(t, universe, 1), originAt(t, universe, 0),
	))
	origins, ok := value.Origins()
	if !ok {
		t.Fatal("finite origins")
	}
	var groups [4]struct {
		source link.Value
		start  int
		end    int
	}
	count := 0
	origins.ForEachSource(func(source link.Value, start, end int) bool {
		if count >= len(groups) {
			t.Fatal("unexpected source group")
		}
		groups[count] = struct {
			source link.Value
			start  int
			end    int
		}{source, start, end}
		count++
		return true
	})
	if count == 0 || count > len(groups) {
		t.Fatalf("source groups=%#v count=%d", groups, count)
	}
	for index := 1; index < count; index++ {
		if groups[index-1].source >= groups[index].source || groups[index-1].end != groups[index].start {
			t.Fatalf("noncanonical source groups=%#v count=%d", groups, count)
		}
	}
	for index := 0; index < 4; index++ {
		entry, present := origins.At(index)
		want := originAt(t, universe, index)
		if !present || origin.Compare(entry, want) != 0 {
			t.Fatalf("origin[%d]=%v/%v, want %#v", index, entry, present, want)
		}
	}
	allocations := testing.AllocsPerRun(100, func() {
		seen := 0
		origins.ForEachSource(func(_ link.Value, start, end int) bool {
			seen += end - start
			return true
		})
		if seen != 4 {
			t.Fatal("source iteration lost origin")
		}
	})
	if allocations != 0 {
		t.Fatalf("source grouping allocated %g times", allocations)
	}
}

func TestCarrierUniverseRejectsOutOfRuleOrigins(t *testing.T) {
	table, integer, _, universe := fixture(t)
	if _, ok := carrier.New(table, universe, integer, origin.New(origin.At(link.Value(0), origin.Fixed(999)))); ok {
		t.Fatal("out-of-universe origin entered carrier")
	}
}

func TestCarrierFiniteUniverseRankAndOperationsExhaustively(t *testing.T) {
	table, integer, text, universe := fixture(t)
	values := exhaustiveValues(t, table, universe, table.Bottom(), integer, text, table.Top())
	for leftIndex, left := range values {
		// An unchanged widening must retain all four rank components exactly.
		unchanged := mustWiden(t, left, left)
		if !carrier.Equal(unchanged, left) || !sameRank(t, unchanged, left) {
			t.Fatalf("equal widening changed value/rank at %d", leftIndex)
		}
		for rightIndex, right := range values {
			joined := mustJoin(t, left, right)
			if !carrier.LessEqual(left, joined) || !carrier.LessEqual(right, joined) {
				t.Fatalf("Join lacks upper bound at %d/%d", leftIndex, rightIndex)
			}
			mu := mustMu(t, left, right)
			if !carrier.Equal(mu, joined) {
				t.Fatalf("Mu differs from exact Join at %d/%d", leftIndex, rightIndex)
			}
			for upperIndex, upper := range values {
				if carrier.LessEqual(left, upper) && carrier.LessEqual(right, upper) && !carrier.LessEqual(joined, upper) {
					t.Fatalf("Join is not least at %d/%d under %d", leftIndex, rightIndex, upperIndex)
				}
			}

			widened := mustWiden(t, left, right)
			if !carrier.LessEqual(left, widened) || !carrier.LessEqual(right, widened) {
				t.Fatalf("Widen lacks upper bound at %d/%d", leftIndex, rightIndex)
			}
			if !carrier.Equal(widened, left) && !rankDescends(t, left, widened) {
				t.Fatalf("Widen rank did not strictly descend at %d/%d: %#v -> %#v", leftIndex, rightIndex, mustRank(t, left), mustRank(t, widened))
			}
		}
	}
}

func TestCarrierRankSeparatesPackOriginsAndFactorTop(t *testing.T) {
	table, integer, text, universe := fixture(t)
	first := originAt(t, universe, 0)
	second := originAt(t, universe, 1)
	packOnlyBefore := mustCarrier(t, table, universe, integer, origin.New(first))
	packOnlyAfter := mustWiden(t, packOnlyBefore, mustCarrier(t, table, universe, text, origin.New(first)))
	assertStrictUpperBoundDescent(t, packOnlyBefore, mustCarrier(t, table, universe, text, origin.New(first)), packOnlyAfter)

	originOnlyBefore := mustCarrier(t, table, universe, integer, origin.New(first))
	originOnlyAfterInput := mustCarrier(t, table, universe, integer, origin.New(first, second))
	originOnlyAfter := mustWiden(t, originOnlyBefore, originOnlyAfterInput)
	assertStrictUpperBoundDescent(t, originOnlyBefore, originOnlyAfterInput, originOnlyAfter)

	simultaneousBefore := mustCarrier(t, table, universe, integer, origin.New(first))
	simultaneousAfterInput := mustCarrier(t, table, universe, text, origin.New(first, second))
	simultaneousAfter := mustWiden(t, simultaneousBefore, simultaneousAfterInput)
	assertStrictUpperBoundDescent(t, simultaneousBefore, simultaneousAfterInput, simultaneousAfter)

	all := make([]origin.Origin, universe.Count())
	for index := range all {
		all[index] = originAt(t, universe, index)
	}
	finiteAtZeroTailRank := mustCarrier(t, table, universe, table.Top(), origin.New(all...))
	top := mustTop(t, table, universe)
	toTop := mustWiden(t, finiteAtZeroTailRank, top)
	assertStrictUpperBoundDescent(t, finiteAtZeroTailRank, top, toTop)
	if rank := mustRank(t, top); rank != (carrier.WidenRank{}) {
		t.Fatalf("FactorTop rank=%#v, want zero", rank)
	}
}

func exhaustiveValues(t testing.TB, table *typedomain.Table, universe *origin.Universe, packs ...typedomain.Pack) []carrier.Value {
	t.Helper()
	if universe.Count() > 8 {
		t.Fatalf("test universe unexpectedly too large: %d", universe.Count())
	}
	result := []carrier.Value{mustTop(t, table, universe)}
	for mask := 0; mask < 1<<universe.Count(); mask++ {
		entries := make([]origin.Origin, 0, universe.Count())
		for index := 0; index < universe.Count(); index++ {
			if mask&(1<<index) != 0 {
				entries = append(entries, originAt(t, universe, index))
			}
		}
		set := origin.New(entries...)
		for _, pack := range packs {
			if pack.IsBottom() && set.Count() != 0 {
				continue
			}
			result = append(result, mustCarrier(t, table, universe, pack, set))
		}
	}
	return result
}

func assertStrictUpperBoundDescent(t testing.TB, previous, next, widened carrier.Value) {
	t.Helper()
	if !carrier.LessEqual(previous, widened) || !carrier.LessEqual(next, widened) {
		t.Fatal("Widen lost upper-bound law")
	}
	if carrier.Equal(previous, widened) || !rankDescends(t, previous, widened) {
		t.Fatalf("Widen did not strictly descend: %#v -> %#v", mustRank(t, previous), mustRank(t, widened))
	}
}

func mustRank(t testing.TB, value carrier.Value) carrier.WidenRank {
	t.Helper()
	rank, ok := value.WidenRank()
	if !ok {
		t.Fatal("carrier rank")
	}
	return rank
}

func sameRank(t testing.TB, left, right carrier.Value) bool {
	t.Helper()
	return mustRank(t, left) == mustRank(t, right)
}

func rankDescends(t testing.TB, before, after carrier.Value) bool {
	t.Helper()
	left, right := mustRank(t, before), mustRank(t, after)
	for _, component := range [][2]uint64{
		{left.NotTop, right.NotTop},
		{left.ShapeClass, right.ShapeClass},
		{left.ExactLabels, right.ExactLabels},
		{left.RemainingOrigins, right.RemainingOrigins},
	} {
		if component[0] != component[1] {
			return component[0] > component[1]
		}
	}
	return false
}

func fixture(t testing.TB) (*typedomain.Table, typedomain.Pack, typedomain.Pack, *origin.Universe) {
	t.Helper()
	table, err := typedomain.NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	integer, err := table.DeriveClosed(typ.Integer)
	if err != nil {
		t.Fatal(err)
	}
	text, err := table.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	table.Seal()
	integerPack, ok := table.Closed(integer)
	if !ok {
		t.Fatal("integer Pack")
	}
	textPack, ok := table.Closed(text)
	if !ok {
		t.Fatal("text Pack")
	}
	return table, integerPack, textPack, carrierUniverse(t)
}

func mustCarrier(t testing.TB, table *typedomain.Table, universe *origin.Universe, pack typedomain.Pack, origins origin.Set) carrier.Value {
	t.Helper()
	if table == nil || !table.Sealed() {
		t.Fatal("test fixture table")
	}
	value, ok := carrier.New(table, universe, pack, origins)
	if !ok {
		t.Fatal("carrier New")
	}
	return value
}

func mustTop(t testing.TB, table *typedomain.Table, universe *origin.Universe) carrier.Value {
	t.Helper()
	value, ok := carrier.Top(table, universe)
	if !ok {
		t.Fatal("carrier Top")
	}
	return value
}

func originAt(t testing.TB, universe *origin.Universe, index int) origin.Origin {
	t.Helper()
	value, ok := universe.At(index)
	if !ok {
		t.Fatalf("universe origin %d absent", index)
	}
	return value
}

func carrierUniverse(t testing.TB) *origin.Universe {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "carrier.lua", Text: []byte(`return 1, "text"`)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "carrier", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	result, ok := origin.Build(source)
	if !ok || result.Count() < 4 {
		t.Fatalf("origin universe=%v/%v, want at least four positions", result, ok)
	}
	return result
}

func mustJoin(t testing.TB, left, right carrier.Value) carrier.Value {
	t.Helper()
	value, ok := carrier.Join(left, right)
	if !ok {
		t.Fatal("carrier Join")
	}
	return value
}

func mustWiden(t testing.TB, previous, next carrier.Value) carrier.Value {
	t.Helper()
	value, ok := carrier.Widen(previous, next)
	if !ok {
		t.Fatal("carrier Widen")
	}
	return value
}

func mustMu(t testing.TB, previous, next carrier.Value) carrier.Value {
	t.Helper()
	value, ok := carrier.Mu(previous, next)
	if !ok {
		t.Fatal("carrier Mu")
	}
	return value
}
