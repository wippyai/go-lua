package acceptance_test

import (
	"testing"

	programlower "github.com/wippyai/go-lua/analysis/lua/lower"
	"github.com/wippyai/go-lua/analysis/program/keyspace"
)

func TestSourceSpellingsPublishBinderNamesForCellsAndCalls(t *testing.T) {
	p, err := programlower.Lower(programlower.Source{
		Name: "spellings.lua",
		Text: []byte(`
local function outer(formal)
  local localValue = formal
  for loopValue = 1, 2 do
    local function inner(...)
      return localValue, ...
    end
    direct()
    object.field()
    object[key]()
    object:method()
  end
end
outer(1)
`),
	})
	if err != nil {
		t.Fatal(err)
	}
	spellings := p.Source().Spellings()
	cells := p.Flow().Authored().Storage().Cells()
	wantCells := map[string]bool{
		"outer": false, "formal": false, "...": false, "localValue": false,
		"loopValue": false, "inner": false, "direct": false, "object": false,
		"key": false,
	}
	for index := 0; index < cells.Count(); index++ {
		cell, ok := cells.At(index)
		if !ok {
			t.Fatalf("CellAt(%d) missing", index)
		}
		name, named := spellings.CellName(cell)
		if named {
			if _, known := wantCells[name]; known {
				wantCells[name] = true
			}
		}
	}
	for name, seen := range wantCells {
		if !seen {
			t.Fatalf("Cell spelling %q was not published", name)
		}
	}

	callNames := map[string]int{}
	dynamic := 0
	calls := p.Flow().Authored().Calls()
	for index := 0; index < calls.Count(); index++ {
		call, ok := calls.At(index)
		if !ok || keyspace.TermFamily(call) != keyspace.FamilyCall {
			t.Fatalf("CallAt(%d) = %v/%v", index, call, ok)
		}
		name, named := spellings.CallName(call)
		if named {
			callNames[name]++
		} else {
			dynamic++
		}
	}
	for _, name := range []string{"outer", "direct", "field", "method"} {
		if callNames[name] == 0 {
			t.Fatalf("published Call spellings omitted %q: %#v", name, callNames)
		}
	}
	if dynamic == 0 {
		t.Fatalf("dynamic/indexed Call unexpectedly received a spelling: %#v", callNames)
	}
}
