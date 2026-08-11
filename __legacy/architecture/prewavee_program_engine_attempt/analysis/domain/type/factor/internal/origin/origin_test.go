package origin_test

import (
	"reflect"
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/program"
	"github.com/wippyai/go-lua/program/link"
	programlower "github.com/wippyai/go-lua/program/lower"
	"github.com/wippyai/go-lua/program/target"
)

func TestPositionAndOriginOrderIsTotalAndCanonical(t *testing.T) {
	positions := []origin.Position{
		origin.Fixed(0), origin.Fixed(1), origin.Tail(0), origin.Tail(1),
	}
	for left := range positions {
		for right := range positions {
			comparison := origin.ComparePosition(positions[left], positions[right])
			inverse := origin.ComparePosition(positions[right], positions[left])
			if sign(comparison) != -sign(inverse) {
				t.Fatalf("position order is not antisymmetric at %d/%d: %d/%d", left, right, comparison, inverse)
			}
			if (left == right) != (comparison == 0) {
				t.Fatalf("position equality mismatch at %d/%d: %d", left, right, comparison)
			}
		}
	}
	for index := 1; index < len(positions); index++ {
		if origin.ComparePosition(positions[index-1], positions[index]) >= 0 {
			t.Fatalf("position sequence not increasing at %d", index)
		}
	}

	left := origin.At(link.Value(2), origin.Tail(0))
	right := origin.At(link.Value(3), origin.Fixed(0))
	if origin.Compare(left, right) >= 0 {
		t.Fatal("source Value must precede position in Origin order")
	}
	if left.Source() != link.Value(2) || !left.Position().IsTail() || left.Position().Index() != 0 {
		t.Fatalf("origin projections changed witness: %#v", left)
	}
}

func TestNewNormalizesAndOwnsItsInput(t *testing.T) {
	input := []origin.Origin{
		origin.At(7, origin.Tail(0)),
		origin.At(2, origin.Tail(1)),
		origin.At(2, origin.Fixed(1)),
		origin.At(2, origin.Fixed(0)),
		origin.At(7, origin.Tail(0)),
	}
	set := origin.New(input...)
	want := []origin.Origin{
		origin.At(2, origin.Fixed(0)),
		origin.At(2, origin.Fixed(1)),
		origin.At(2, origin.Tail(1)),
		origin.At(7, origin.Tail(0)),
	}
	assertEntries(t, set, want)

	// New owns its normalized result rather than retaining the caller's input
	// slice.  A carrier can therefore safely retain the resulting Set.
	input[0] = origin.At(0, origin.Fixed(99))
	assertEntries(t, set, want)
	if origin.New().Count() != 0 || !origin.New().Equal(origin.Empty()) {
		t.Fatal("empty normalization is not canonical")
	}
}

func TestSetOperationsSatisfyFiniteSetLaws(t *testing.T) {
	universe := []origin.Origin{
		origin.At(0, origin.Fixed(0)),
		origin.At(0, origin.Tail(0)),
		origin.At(1, origin.Fixed(1)),
		origin.At(4, origin.Fixed(0)),
	}
	sets := make([]origin.Set, 1<<len(universe))
	for mask := range sets {
		entries := make([]origin.Origin, 0, len(universe))
		for index, value := range universe {
			if mask&(1<<index) != 0 {
				entries = append(entries, value)
			}
		}
		sets[mask] = origin.New(entries...)
	}
	for leftMask, left := range sets {
		if !origin.Union(left, origin.Empty()).Equal(left) ||
			!origin.Intersect(left, origin.Empty()).Equal(origin.Empty()) ||
			!origin.Difference(left, origin.Empty()).Equal(left) {
			t.Fatalf("empty identity failed for mask %b", leftMask)
		}
		for rightMask, right := range sets {
			union := origin.Union(left, right)
			intersection := origin.Intersect(left, right)
			difference := origin.Difference(left, right)
			if !union.Equal(origin.Union(right, left)) || !intersection.Equal(origin.Intersect(right, left)) {
				t.Fatalf("commutativity failed for masks %b/%b", leftMask, rightMask)
			}
			if !left.LessEqual(union) || !right.LessEqual(union) || !intersection.LessEqual(left) || !intersection.LessEqual(right) {
				t.Fatalf("subset bounds failed for masks %b/%b", leftMask, rightMask)
			}
			for _, value := range universe {
				if union.Contains(value) != (left.Contains(value) || right.Contains(value)) {
					t.Fatalf("union membership failed for masks %b/%b", leftMask, rightMask)
				}
				if intersection.Contains(value) != (left.Contains(value) && right.Contains(value)) {
					t.Fatalf("intersection membership failed for masks %b/%b", leftMask, rightMask)
				}
				if difference.Contains(value) != (left.Contains(value) && !right.Contains(value)) {
					t.Fatalf("difference membership failed for masks %b/%b", leftMask, rightMask)
				}
			}
			for _, third := range sets {
				if !origin.Union(origin.Union(left, right), third).Equal(origin.Union(left, origin.Union(right, third))) {
					t.Fatalf("union associativity failed for masks %b/%b", leftMask, rightMask)
				}
			}
		}
	}
}

func TestForEachSourceUsesContiguousCanonicalRanges(t *testing.T) {
	set := origin.New(
		origin.At(4, origin.Tail(0)),
		origin.At(1, origin.Tail(1)),
		origin.At(1, origin.Fixed(0)),
		origin.At(4, origin.Fixed(2)),
		origin.At(9, origin.Fixed(0)),
	)
	type group struct {
		source link.Value
		items  []origin.Origin
	}
	var groups []group
	set.ForEachSource(func(source link.Value, start, end int) bool {
		items := make([]origin.Origin, 0, end-start)
		for index := start; index < end; index++ {
			item, ok := set.At(index)
			if !ok || item.Source() != source {
				t.Fatalf("invalid range %d:%d for source %d", start, end, source)
			}
			items = append(items, item)
		}
		groups = append(groups, group{source: source, items: items})
		return true
	})
	want := []group{
		{source: 1, items: []origin.Origin{origin.At(1, origin.Fixed(0)), origin.At(1, origin.Tail(1))}},
		{source: 4, items: []origin.Origin{origin.At(4, origin.Fixed(2)), origin.At(4, origin.Tail(0))}},
		{source: 9, items: []origin.Origin{origin.At(9, origin.Fixed(0))}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Fatalf("groups=%#v, want %#v", groups, want)
	}
	visited := 0
	set.ForEachSource(func(link.Value, int, int) bool {
		visited++
		return false
	})
	if visited != 1 {
		t.Fatalf("early stop visited %d sources", visited)
	}
}

func TestForEachSourceAllocatesNothing(t *testing.T) {
	set := origin.New(
		origin.At(0, origin.Fixed(0)), origin.At(0, origin.Tail(0)),
		origin.At(1, origin.Fixed(0)), origin.At(3, origin.Fixed(2)),
	)
	seen := 0
	visit := func(_ link.Value, start, end int) bool {
		seen += end - start
		return true
	}
	allocations := testing.AllocsPerRun(1_000, func() {
		set.ForEachSource(visit)
	})
	if allocations != 0 {
		t.Fatalf("source iteration allocations=%f, want 0", allocations)
	}
	if seen == 0 {
		t.Fatal("source iteration did not execute")
	}
}

func TestUniverseClosesOnlyCurrentTypeRulePositions(t *testing.T) {
	source := linked(t, `
local function variadic(...) return ... end
return 1, "text", variadic(...)
`)
	universe, ok := origin.Build(source)
	if !ok {
		t.Fatal("build origin universe")
	}
	if universe.Count() == 0 {
		t.Fatal("literal/Values Type rules produced an empty provenance universe")
	}

	shard, ok := source.ShardAt(0)
	if !ok {
		t.Fatal("source shard")
	}
	p, ok := source.Program(shard)
	if !ok || p == nil {
		t.Fatal("source Program")
	}
	for _, literal := range []struct {
		count int
		at    func(int) (program.Term, bool)
	}{
		{p.NilCount(), p.NilAt}, {p.BoolCount(), p.BoolAt},
		{p.IntegerCount(), p.IntegerAt}, {p.FloatCount(), p.FloatAt}, {p.StringCount(), p.StringAt},
	} {
		for index := 0; index < literal.count; index++ {
			term, found := literal.at(index)
			if !found {
				t.Fatal("malformed literal family")
			}
			value, found := source.ValueOf(shard, term)
			if !found || !universe.Contains(origin.At(value, origin.Fixed(0))) {
				t.Fatalf("literal %d lacks admitted scalar origin", term)
			}
			if universe.Contains(origin.At(value, origin.Fixed(1))) {
				t.Fatalf("literal %d admitted nonexistent second result", term)
			}
		}
	}

	seenTail := false
	for index := 0; index < p.ValuesCount(); index++ {
		term, found := p.ValuesAt(index)
		if !found {
			t.Fatal("malformed Values family")
		}
		value, found := source.ValueOf(shard, term)
		if !found {
			t.Fatal("Values has no Link Value")
		}
		fixed, found := p.ValuesLen(term)
		if !found {
			t.Fatal("Values length")
		}
		for ordinal := 0; ordinal < fixed; ordinal++ {
			if !universe.Contains(origin.At(value, origin.Fixed(uint32(ordinal)))) {
				t.Fatalf("Values %d fixed position %d absent", term, ordinal)
			}
		}
		if universe.Contains(origin.At(value, origin.Fixed(uint32(fixed)))) {
			t.Fatalf("Values %d admitted nonexistent fixed position %d", term, fixed)
		}
		_, tail, found := p.Values(term)
		if !found {
			t.Fatal("Values row")
		}
		if tail != 0 {
			seenTail = true
			if !universe.Contains(origin.At(value, origin.Tail(0))) {
				t.Fatalf("Values %d tail position absent", term)
			}
			if universe.Contains(origin.At(value, origin.Tail(1))) {
				t.Fatalf("Values %d admitted nonexistent second tail witness", term)
			}
		} else if universe.Contains(origin.At(value, origin.Tail(0))) {
			t.Fatalf("closed Values %d admitted tail witness", term)
		}
	}
	if !seenTail {
		t.Fatal("fixture failed to expose a variadic Values rule")
	}

	// A Link Value alone has no meaning across project authorities.  This
	// forged ordinal is rejected because it is not one of this Link's closed
	// Type-rule origins.
	if universe.Valid(origin.New(origin.At(link.Value(0), origin.Fixed(999)))) {
		t.Fatal("out-of-universe provenance was accepted")
	}
}

func linked(t testing.TB, text string) *link.Link {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "origin.lua", Text: []byte(text)})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "origin", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func assertEntries(t testing.TB, set origin.Set, want []origin.Origin) {
	t.Helper()
	if set.Count() != len(want) {
		t.Fatalf("set count=%d, want %d", set.Count(), len(want))
	}
	for index, expected := range want {
		actual, ok := set.At(index)
		if !ok || origin.Compare(actual, expected) != 0 {
			t.Fatalf("set[%d]=%#v/%v, want %#v", index, actual, ok, expected)
		}
	}
}

func sign(value int) int {
	switch {
	case value < 0:
		return -1
	case value > 0:
		return 1
	default:
		return 0
	}
}
