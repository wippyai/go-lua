package visibility

import (
	"fmt"
	"testing"

	"github.com/wippyai/go-lua/analysis/ir/ssa"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestVersionStateIsUnboundedAndPersistent(t *testing.T) {
	const symbols = 4096
	var state *versionState
	for i := 1; i <= symbols; i++ {
		sym := symbol.ID(i)
		state = state.with(ssa.Version{Root: fmt.Sprintf("v%d", i), Symbol: sym, ID: 1})
	}
	if state.count != symbols {
		t.Fatalf("state count = %d, want %d", state.count, symbols)
	}
	for i := 1; i <= symbols; i++ {
		sym := symbol.ID(i)
		if got := state.lookup(sym); got.Symbol != sym || got.ID != 1 {
			t.Fatalf("lookup(%d) = %+v, want symbol %d version 1", sym, got, sym)
		}
	}

	changedSymbol := symbol.ID(symbols / 2)
	changed := state.with(ssa.Version{Root: "changed", Symbol: changedSymbol, ID: 2})
	if got := changed.lookup(changedSymbol); got.ID != 2 {
		t.Fatalf("changed lookup = %+v, want version 2", got)
	}
	if got := state.lookup(changedSymbol); got.ID != 1 {
		t.Fatalf("original lookup after persistent update = %+v, want version 1", got)
	}
	if changed.count != state.count {
		t.Fatalf("changed count = %d, want %d", changed.count, state.count)
	}
}
