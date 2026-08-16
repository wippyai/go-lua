package values

import (
	"testing"

	typedomain "github.com/wippyai/go-lua/analysis/domain/type"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/carrier"
	"github.com/wippyai/go-lua/analysis/domain/type/factor/internal/origin"
	"github.com/wippyai/go-lua/analysis/domain/type/typ"
	"github.com/wippyai/go-lua/analysis/program/link"
	programlower "github.com/wippyai/go-lua/analysis/program/lower"
	"github.com/wippyai/go-lua/analysis/program/target"
)

// Current Program ownership makes Values operands unique, but a future
// Cell/Read transfer may name one Link Value in several slots. This law keeps
// the cold Equation layout honest independently of that future source form:
// one distinct source has one group/read and every repeated slot names it.
func TestGroupedLayoutDeduplicatesRepeatedSourceAndReadsItOnce(t *testing.T) {
	const (
		left  = link.Value(3)
		right = link.Value(7)
	)
	groups := canonicalGroups([]link.Value{right, left, right, left})
	if len(groups) != 2 || groups[0] != left || groups[1] != right {
		t.Fatalf("canonical groups = %#v, want [%d %d]", groups, left, right)
	}
	leftGroup, ok := groupOf(groups, left)
	if !ok {
		t.Fatal("left group")
	}
	rightGroup, ok := groupOf(groups, right)
	if !ok {
		t.Fatal("right group")
	}
	equation := Equation{
		result:      link.Value(9),
		groups:      groups,
		fixedGroups: []uint32{rightGroup, leftGroup, rightGroup},
		tailGroup:   rightGroup,
		hasTail:     true,
	}
	for slot, want := range []link.Value{right, left, right} {
		got, present := equation.FixedAt(slot)
		if !present || got != want {
			t.Fatalf("fixed slot %d = %d/%v, want %d", slot, got, present, want)
		}
	}
	if got, present := equation.Tail(); !present || got != right {
		t.Fatalf("tail = %d/%v, want %d", got, present, right)
	}

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
	empty, ok := table.Closed()
	if !ok {
		t.Fatal("closed empty")
	}
	integerPack, ok := table.Closed(integer)
	if !ok {
		t.Fatal("integer pack")
	}
	textPack, ok := table.Closed(text)
	if !ok {
		t.Fatal("text pack")
	}
	universe := groupedLayoutUniverse(t)
	seen := make([]bool, len(groups))
	reads := 0
	value, ok := equation.Evaluate(table, universe, empty, func(index int, key link.Value) (carrier.Value, bool) {
		if index < 0 || index >= len(groups) || seen[index] || groups[index] != key {
			t.Fatalf("read = %d/%d, groups=%#v, seen=%v", index, key, groups, seen)
		}
		seen[index] = true
		reads++
		pack := integerPack
		if key == right {
			pack = textPack
		}
		result, valid := carrier.New(table, universe, pack, origin.Empty())
		return result, valid
	}, make([]typedomain.Pack, len(groups)))
	if !ok || reads != len(groups) || !seen[0] || !seen[1] {
		t.Fatalf("group reads = %d/%v, valid=%v", reads, seen, ok)
	}
	if origins, finite := value.Origins(); !finite || origins.Count() != 0 {
		t.Fatalf("Values origins = %d/%v, want empty", origins.Count(), finite)
	}
}

func groupedLayoutUniverse(t testing.TB) *origin.Universe {
	t.Helper()
	p, err := programlower.Lower(programlower.Source{Name: "grouped-layout.lua", Text: []byte("return 0")})
	if err != nil {
		t.Fatal(err)
	}
	contract, err := target.Seal(&target.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	source, err := link.Seal(&link.Spec{Target: contract, Modules: []link.Module{{Name: "grouped-layout", Program: p}}})
	if err != nil {
		t.Fatal(err)
	}
	universe, ok := origin.Build(source)
	if !ok {
		t.Fatal("origin universe")
	}
	return universe
}
