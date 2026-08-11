package typedomain

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/type/typ"
)

func TestPackUsesOnlyTableLocalFlatLabels(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.Integer, typ.Number, typ.String)
	integer, number, text := labels[0], labels[1], labels[2]
	table.Seal()

	integerPack, ok := table.Closed(integer)
	if !ok {
		t.Fatal("integer pack")
	}
	numberPack, ok := table.Closed(number)
	if !ok {
		t.Fatal("number pack")
	}
	textPack, ok := table.Closed(text)
	if !ok {
		t.Fatal("text pack")
	}
	topPack, ok := table.Closed(table.TypeTop())
	if !ok {
		t.Fatal("top-label pack")
	}
	// Integer is a subtype of number in the cold typ algebra, but Pack must
	// not use that relation. Only exact labels and the one TypeTop label order
	// are valid in this hot carrier.
	if LessEqual(integerPack, numberPack) {
		t.Fatal("pack leaked Table/cold subtype algebra")
	}
	if !LessEqual(integerPack, topPack) || LessEqual(topPack, integerPack) {
		t.Fatal("flat TypeTop order is wrong")
	}

	joined, ok := Join(integerPack, textPack)
	if !ok || len(joined.Modes()) != 2 {
		t.Fatalf("correlated join=%#v / %v", joined.Modes(), ok)
	}
	if modes := joined.Modes(); len(modes) != 2 {
		t.Fatal("mode view")
	} else {
		prefix := modes[0].Prefix()
		if len(prefix) != 1 {
			t.Fatalf("prefix=%v", prefix)
		}
		prefix[0] = table.Nil()
		if joined.Modes()[0].Prefix()[0] == table.Nil() {
			t.Fatal("mode view mutated retained fact")
		}
	}

	other := newPackTable(t)
	foreign, err := other.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := table.Closed(foreign); ok {
		t.Fatal("foreign handle entered pack")
	}
	other.Seal()
	if otherPack, ok := other.Closed(foreign); !ok {
		t.Fatal("other pack")
	} else if _, ok := Join(integerPack, otherPack); ok {
		t.Fatal("cross-table pack join succeeded")
	}
}

func TestPackMuWidenAndFixedProjection(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String, typ.Number, typ.Boolean)
	a, b, c := labels[0], labels[1], labels[2]
	table.Seal()

	previous, ok := table.Pack(ClosedMode(a, b), ClosedMode(b, a))
	if !ok {
		t.Fatal("previous")
	}
	next, ok := table.Pack(ClosedMode(a, c), ClosedMode(c, a))
	if !ok {
		t.Fatal("next")
	}
	widened, ok := Widen(previous, next)
	if !ok || widened.IsTop() {
		t.Fatalf("widen=%#v / %v", widened.Modes(), ok)
	}
	widenMode := widened.Modes()
	if len(widenMode) != 1 || widenMode[0].Kind() != ModeClosed || len(widenMode[0].Prefix()) != 2 {
		t.Fatalf("widen modes=%#v", widenMode)
	}
	for _, label := range widenMode[0].Prefix() {
		if label != table.TypeTop() {
			t.Fatalf("varying label was not TypeTop: %#v", widenMode)
		}
	}
	before, after := previous.WidenRank(), widened.WidenRank()
	if !(after.ShapeClass < before.ShapeClass || after.ShapeClass == before.ShapeClass && after.ExactLabels < before.ExactLabels) {
		t.Fatalf("rank=%#v -> %#v", before, after)
	}

	final, ok := table.Open(nil, b, []Handle{c})
	if !ok {
		t.Fatal("final")
	}
	fixed, ok := FixedAt(final, 3, 1)
	if !ok || len(fixed.Modes()) == 0 {
		t.Fatalf("fixed position=%#v / %v", fixed.Modes(), ok)
	}
}

func TestPackFixedAtIsDemandOnly(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String, typ.Number, typ.Boolean)
	a, b, c := labels[0], labels[1], labels[2]
	table.Seal()
	value, ok := table.Open([]Handle{a}, b, []Handle{c})
	if !ok {
		t.Fatal("open pack")
	}
	if _, ok := FixedAt(value, 0, 0); ok {
		t.Fatal("zero-width position accepted")
	}
	allocations := func(width int) float64 {
		return testing.AllocsPerRun(50, func() {
			if _, ok := FixedAt(value, width, 0); !ok {
				t.Fatal("valid demanded position rejected")
			}
		})
	}
	short, wide := allocations(1), allocations(1<<20)
	if wide > short+1 {
		t.Fatalf("FixedAt low index allocated by width: width=1 %g, width=1<<20 %g", short, wide)
	}
}

func TestPackAssembleHasOneSealedTableAuthority(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String, typ.Number)
	a, b := labels[0], labels[1]
	table.Seal()
	fixed, ok := table.Pack(ClosedMode(a), ClosedMode(b))
	if !ok {
		t.Fatal("fixed alternatives")
	}
	empty, ok := table.Closed()
	if !ok {
		t.Fatal("closed empty final")
	}
	assembled, ok := Assemble([]Pack{fixed}, empty)
	if !ok || assembled.IsBottom() {
		t.Fatalf("sealed assembly=%#v / %v", assembled.Modes(), ok)
	}

	other := newPackTable(t)
	foreign, err := other.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	other.Seal()
	otherFixed, ok := other.Closed(foreign)
	if !ok {
		t.Fatal("foreign fixed")
	}
	if _, ok := Assemble([]Pack{otherFixed}, empty); ok {
		t.Fatal("foreign fixed Pack entered assembly")
	}
	if _, ok := Assemble(nil, Pack{}); ok {
		t.Fatal("zero final Pack entered assembly")
	}

	unsealed := newPackTable(t)
	if _, ok := Assemble(nil, unsealed.Bottom()); ok {
		t.Fatal("unsealed final Pack entered assembly")
	}
}

func TestPackAssembleBottomTailDoesNotMaterializeFixedValueSlice(t *testing.T) {
	// A bottom final tail makes the Values relation bottom before it needs any
	// fixed operand.  This probes the outer Pack boundary specifically: the
	// old path first allocated and copied []sequence.Value even though the
	// sequence carrier could return immediately.
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String)
	table.Seal()
	unit, ok := table.Closed(labels[0])
	if !ok {
		t.Fatal("unit")
	}
	fixed := make([]Pack, 10_000)
	for index := range fixed {
		fixed[index] = unit
	}
	bottom := table.Bottom()
	if allocations := testing.AllocsPerRun(50, func() {
		value, valid := Assemble(fixed, bottom)
		if !valid || !value.IsBottom() {
			t.Fatal("bottom final lost")
		}
	}); allocations != 0 {
		t.Fatalf("bottom Assemble copied fixed operands: allocations=%g", allocations)
	}
}

func TestPackAssembleGroupedUsesOneCanonicalOwnerAndBottomShortCircuit(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String)
	table.Seal()
	empty, ok := table.Closed()
	if !ok {
		t.Fatal("empty")
	}
	bottom := table.Bottom()
	allocations := func(width int) float64 {
		fixedGroups := make([]uint32, width)
		return testing.AllocsPerRun(50, func() {
			value, valid := AssembleGrouped([]Pack{bottom}, fixedGroups, 0, false, empty)
			if !valid || !value.IsBottom() {
				t.Fatal("bottom grouped assembly")
			}
		})
	}
	short, wide := allocations(1), allocations(10_000)
	if wide > short+1 {
		t.Fatalf("bottom grouped assembly materialized slot state: width=1 %g, width=10000 %g", short, wide)
	}
	if value, valid := AssembleGrouped([]Pack{bottom}, nil, 0, false, empty); !valid || !value.IsBottom() {
		// A zero-slot bottom route remains valid independently of the measured
		// widths above.
		t.Fatal("zero-slot bottom grouped assembly")
	}

	unit, ok := table.Closed(labels[0])
	if !ok {
		t.Fatal("unit")
	}
	foreignTable := newPackTable(t)
	foreign, err := foreignTable.DeriveClosed(typ.String)
	if err != nil {
		t.Fatal(err)
	}
	foreignTable.Seal()
	foreignPack, ok := foreignTable.Closed(foreign)
	if !ok {
		t.Fatal("foreign Pack")
	}
	if _, ok := AssembleGrouped([]Pack{foreignPack}, []uint32{0}, 0, false, empty); ok {
		t.Fatal("foreign group entered grouped assembly")
	}
	if _, ok := AssembleGrouped([]Pack{unit}, []uint32{1}, 0, false, empty); ok {
		t.Fatal("out-of-range fixed group accepted")
	}
	if _, ok := AssembleGrouped([]Pack{unit}, nil, 1, true, empty); ok {
		t.Fatal("out-of-range tail group accepted")
	}
	if _, ok := AssembleGrouped([]Pack{unit}, nil, 0, false, unit); ok {
		t.Fatal("non-empty absent-tail base accepted")
	}
}

func BenchmarkPackAssembleBottomTail10000(b *testing.B) {
	table := newPackTable(b)
	labels := derivePackLabels(b, table, typ.String)
	table.Seal()
	unit, ok := table.Closed(labels[0])
	if !ok {
		b.Fatal("unit")
	}
	fixed := make([]Pack, 10_000)
	for index := range fixed {
		fixed[index] = unit
	}
	bottom := table.Bottom()
	b.ReportAllocs()
	for range b.N {
		if _, ok := Assemble(fixed, bottom); !ok {
			b.Fatal("assemble")
		}
	}
}

func TestPackHotIdentityPathsAllocateNothing(t *testing.T) {
	table := newPackTable(t)
	labels := derivePackLabels(t, table, typ.String, typ.Number)
	a, b := labels[0], labels[1]
	table.Seal()
	value, ok := table.Pack(ClosedMode(a, b), KnownMode(nil, a, []Handle{b}))
	if !ok {
		t.Fatal("value")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		if !Equal(value, value) || !LessEqual(value, value) {
			t.Fatal("identity relation")
		}
		if joined, ok := Join(value, value); !ok || !Equal(joined, value) {
			t.Fatal("identity join")
		}
		if widened, ok := Widen(value, value); !ok || !Equal(widened, value) {
			t.Fatal("identity widen")
		}
		_ = value.Hash()
	}); allocations != 0 {
		t.Fatalf("hot identity allocations=%g", allocations)
	}
}

func newPackTable(t testing.TB) *Table {
	t.Helper()
	table, err := NewTable(nil)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func derivePackLabels(t testing.TB, table *Table, values ...typ.Type) []Handle {
	t.Helper()
	handles := make([]Handle, len(values))
	for index, value := range values {
		var err error
		handles[index], err = table.DeriveClosed(value)
		if err != nil {
			t.Fatalf("derive %T: %v", value, err)
		}
	}
	return handles
}
