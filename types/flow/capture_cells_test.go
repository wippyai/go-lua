package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestCaptureCellsLatticeLaws(t *testing.T) {
	n := product.FromType(typ.Number)
	s := product.FromType(typ.String)
	optN := product.FromType(typ.NewOptional(typ.Number))

	lattice.LawSuite[CaptureCells]{
		Name:   "CaptureCells",
		Domain: CaptureCellsDomain,
		Sample: []CaptureCells{
			CaptureCellsDomain.Bottom(),
			CaptureCellsDomain.Top(),
			CaptureCellsOf([]CaptureCell{{Symbol: 1, Value: n}}),
			CaptureCellsOf([]CaptureCell{{Symbol: 2, Value: s}}),
			CaptureCellsOf([]CaptureCell{{Symbol: 1, Value: n}, {Symbol: 2, Value: s}}),
			CaptureCellsOf([]CaptureCell{{Symbol: 1, Value: optN}}),
		},
		Format: func(c CaptureCells) string { return c.Format() },
	}.Run(t)
}

func TestCaptureCellsCanonicalization(t *testing.T) {
	got := CaptureCellsOf([]CaptureCell{
		{Symbol: 2, Value: product.FromType(typ.String)},
		{Symbol: 1, Value: product.Domain.Bottom()},
		{Symbol: 2, Value: product.FromType(typ.Number)},
		{Symbol: 0, Value: product.FromType(typ.Boolean)},
	})

	entries := got.Entries()
	if len(entries) != 1 {
		t.Fatalf("entries len = %d, want 1: %s", len(entries), got.Format())
	}
	if entries[0].Symbol != cfg.SymbolID(2) {
		t.Fatalf("symbol = %d, want 2", entries[0].Symbol)
	}
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Number))
	if !product.Domain.Equal(entries[0].Value, want) {
		t.Fatalf("merged value = %s, want %s", entries[0].Value.ProjectValue(), want.ProjectValue())
	}
}

func TestCaptureCellsWithStrongUpdate(t *testing.T) {
	c := CaptureCellsOf([]CaptureCell{{Symbol: 1, Value: product.FromType(typ.Number)}})
	c = c.With(1, product.FromType(typ.String))
	got, ok := c.Value(1)
	if !ok || !product.Domain.Equal(got, product.FromType(typ.String)) {
		t.Fatalf("cell 1 = %v/%v, want string", got.ProjectValue(), ok)
	}

	c = c.With(1, product.Domain.Bottom())
	if _, ok := c.Value(1); ok {
		t.Fatalf("bottom update should remove cell: %s", c.Format())
	}
	if !CaptureCellsDomain.Equal(c, CaptureCellsDomain.Bottom()) {
		t.Fatalf("after removing last cell = %s, want bottom", c.Format())
	}
}

func TestCaptureCellsProjectCanonicalizesSymbols(t *testing.T) {
	c := CaptureCellsOf([]CaptureCell{
		{Symbol: 1, Value: product.FromType(typ.Number)},
		{Symbol: 2, Value: product.FromType(typ.String)},
		{Symbol: 3, Value: product.FromType(typ.Boolean)},
	})

	got := c.Project([]cfg.SymbolID{3, 1, 3, 0})
	entries := got.Entries()
	if len(entries) != 2 || entries[0].Symbol != 1 || entries[1].Symbol != 3 {
		t.Fatalf("project entries = %v; store=%s", entries, got.Format())
	}
	if _, ok := got.Value(2); ok {
		t.Fatalf("project retained unrequested symbol 2: %s", got.Format())
	}
}

func TestCaptureCellsKeyInternsByExactEquality(t *testing.T) {
	a := CaptureCellsOf([]CaptureCell{
		{Symbol: 2, Value: product.FromType(typ.String)},
		{Symbol: 1, Value: product.FromType(typ.Number)},
	})
	b := CaptureCellsOf([]CaptureCell{
		{Symbol: 1, Value: product.FromType(typ.Number)},
		{Symbol: 2, Value: product.FromType(typ.String)},
	})
	c := CaptureCellsOf([]CaptureCell{{Symbol: 1, Value: product.FromType(typ.Number)}})

	if a.Key() != b.Key() {
		t.Fatalf("equal cells produced different keys: %s vs %s", a.Format(), b.Format())
	}
	if a.Key() == c.Key() {
		t.Fatalf("distinct cells produced same key: %s vs %s", a.Format(), c.Format())
	}
	if !CaptureCellsDomain.Equal(a.Key().Cells(), a) {
		t.Fatalf("key roundtrip = %s, want %s", a.Key().Cells().Format(), a.Format())
	}
}

func TestCaptureCellsJoinAndWidenArePointwise(t *testing.T) {
	left := CaptureCellsOf([]CaptureCell{
		{Symbol: 1, Value: product.FromType(typ.Number)},
		{Symbol: 3, Value: product.FromType(typ.Boolean)},
	})
	right := CaptureCellsOf([]CaptureCell{
		{Symbol: 1, Value: product.FromType(typ.String)},
		{Symbol: 2, Value: product.FromType(typ.Nil)},
	})

	joined := CaptureCellsDomain.Join(left, right)
	if len(joined.Entries()) != 3 {
		t.Fatalf("joined entries = %s, want three symbols", joined.Format())
	}
	if v, ok := joined.Value(1); !ok || !product.Domain.Equal(v, product.Domain.Join(product.FromType(typ.Number), product.FromType(typ.String))) {
		t.Fatalf("joined cell 1 = %v/%v", v.ProjectValue(), ok)
	}
	if widened := CaptureCellsDomain.Widen(left, right); !CaptureCellsDomain.LessOrEq(joined, widened) {
		t.Fatalf("widen must over-approximate join: join=%s widen=%s", joined.Format(), widened.Format())
	}
}
