package program_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestTransformerReturnReceiptUsesDenseExecutableDenominator(t *testing.T) {
	published, err := lualower.Lower(lualower.Source{Name: "transformer-return-receipt.lua", Text: []byte(`
local function return_dead()
  do return 1 end
  local hidden = 2
  return hidden
end
return return_dead
`)})
	if err != nil {
		t.Fatalf("Lower: %v", err)
	}
	input := published.TransformerInput()
	authored := published.Flow().Authored().Control().Returns()
	want := 0
	for index := 0; index < authored.Count(); index++ {
		term, ok := authored.At(index)
		if !ok {
			t.Fatalf("authored ReturnAt(%d)", index)
		}
		if published.Flow().Executable().Contains(term) {
			want++
		}
	}
	if want >= authored.Count() {
		t.Fatalf("fixture did not retain a dead authored Return: executable/authored=%d/%d", want, authored.Count())
	}
	if got := input.ReturnOccurrenceCount(); got != want {
		t.Fatalf("ReturnOccurrenceCount=%d, want exact executable denominator %d", got, want)
	}
	for index := 0; index < input.ReturnOccurrenceCount(); index++ {
		row, ok := input.ReturnOccurrenceAt(index)
		if !ok || !row.Available() || !row.ContextID().Available() || !row.ValuesID().Available() {
			t.Fatalf("ReturnOccurrenceAt(%d) did not issue a complete sealed receipt", index)
		}
	}
}
