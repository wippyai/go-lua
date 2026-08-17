package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestLocalQueryReturnsParentIssuedWTOEvents(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "local-wto.lua",
		Text: []byte("local value = 2\nwhile value > 0 do value = value - 1 end\nreturn value"),
	})
	if err != nil {
		t.Fatal(err)
	}
	wto := program.Flow().Local().WTO()
	if wto.Count() == 0 || wto.EventCount() == 0 {
		t.Fatalf("Local WTO = regions %d/events %d, want the sealed schedule", wto.Count(), wto.EventCount())
	}
	for index := 0; index < wto.EventCount(); index++ {
		event, ok := wto.EventAt(index)
		if !ok || !event.Available() {
			t.Fatalf("WTO.EventAt(%d) = %#v/%v, want an issued event", index, event, ok)
		}
	}
}
