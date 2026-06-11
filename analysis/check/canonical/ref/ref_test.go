package ref

import (
	"fmt"
	"sort"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/cfg"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestFuncRefZero(t *testing.T) {
	var zero FuncRef
	if !zero.IsZero() {
		t.Fatalf("zero FuncRef should report zero")
	}
	if got := Zero(); got != zero {
		t.Fatalf("Zero() = %#v, want %#v", got, zero)
	}
	if got := FromCFG(nil); got != zero {
		t.Fatalf("FromCFG(nil) = %#v, want zero", got)
	}
	if got := FromSymbol(0); got != zero {
		t.Fatalf("FromSymbol(0) = %#v, want zero", got)
	}
	if got := zero.String(); got != "func:zero" {
		t.Fatalf("zero String() = %q", got)
	}
}

func TestFuncRefConstructorsAndString(t *testing.T) {
	g := cfg.New()
	cfgRef := FromCFG(g)
	if cfgRef.Kind != KindCFG || cfgRef.ID != g.ID() {
		t.Fatalf("FromCFG() = %#v, want cfg id %d", cfgRef, g.ID())
	}
	if got, want := cfgRef.String(), fmt.Sprintf("func:cfg:%d", g.ID()); got != want {
		t.Fatalf("cfg String() = %q", got)
	}

	symRef := FromSymbol(symbol.ID(42))
	if symRef != (FuncRef{Kind: KindSymbol, ID: 42}) {
		t.Fatalf("FromSymbol(42) = %#v", symRef)
	}
	if got := symRef.String(); got != "func:symbol:42" {
		t.Fatalf("symbol String() = %q", got)
	}
}

func TestFuncRefComparableAndDeterministicOrdering(t *testing.T) {
	seen := map[FuncRef]string{
		{}:                         "zero",
		{Kind: KindCFG, ID: 2}:     "cfg2",
		{Kind: KindCFG, ID: 1}:     "cfg1",
		{Kind: KindSymbol, ID: 1}:  "sym1",
		{Kind: KindSymbol, ID: 10}: "sym10",
	}
	if seen[FuncRef{}] != "zero" {
		t.Fatalf("FuncRef is not usable as expected map key")
	}

	refs := []FuncRef{
		{Kind: KindSymbol, ID: 10},
		{Kind: KindCFG, ID: 2},
		{},
		{Kind: KindSymbol, ID: 1},
		{Kind: KindCFG, ID: 1},
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].Less(refs[j]) })

	want := []FuncRef{
		{},
		{Kind: KindCFG, ID: 1},
		{Kind: KindCFG, ID: 2},
		{Kind: KindSymbol, ID: 1},
		{Kind: KindSymbol, ID: 10},
	}
	for i := range want {
		if refs[i] != want[i] {
			t.Fatalf("refs[%d] = %#v, want %#v", i, refs[i], want[i])
		}
	}
}
