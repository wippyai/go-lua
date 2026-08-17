package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestDerivedOutcomeQueriesResolveAuthoredReturn(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "derived-outcome.lua",
		Text: []byte("return 1"),
	})
	if err != nil {
		t.Fatal(err)
	}
	view := program.Flow()
	entry, ok := program.Source().Index().Entry()
	if !ok {
		t.Fatal("missing Source entry Body")
	}
	start, end, ok := view.Outcomes().BodyRange(entry)
	if !ok || end <= start {
		t.Fatalf("BodyRange(%v) = %d/%d/%v, want the entry outcome range", entry, start, end, ok)
	}
	for index := start; index < end; index++ {
		term, ok := view.Outcomes().At(index)
		if !ok {
			t.Fatalf("Outcomes.At(%d) failed inside the body range", index)
		}
		outcome, ok := view.Outcomes().Get(term)
		if !ok || outcome.Body != entry {
			t.Fatalf("Outcomes.Get(%v) = %#v/%v, want entry-owned outcome", term, outcome, ok)
		}
	}
}
