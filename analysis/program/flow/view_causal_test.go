package flow_test

import (
	"testing"

	lualower "github.com/wippyai/go-lua/analysis/lua/lower"
)

func TestCausalQueriesExposeSealedLoopRoutes(t *testing.T) {
	program, err := lualower.Lower(lualower.Source{
		Name: "causal-query.lua",
		Text: []byte("local value = 2\nwhile value > 0 do value = value - 1 end\nreturn value"),
	})
	if err != nil {
		t.Fatal(err)
	}
	causal := program.Flow().Causal()
	if causal.Edges().Count() == 0 || causal.Successors().TotalCount() == 0 {
		t.Fatalf("causal rows = edges %d, successors %d; want the loop transfer relation", causal.Edges().Count(), causal.Successors().TotalCount())
	}
	for index := 0; index < causal.Successors().TotalCount(); index++ {
		successor, ok := causal.Successors().TotalAt(index)
		if !ok {
			t.Fatalf("Successors.TotalAt(%d) failed inside its denominator", index)
		}
		identity, ok := successor.Identity()
		if !ok || !identity.Available() {
			t.Fatalf("successor[%d] identity = %#v/%v, want a sealed route identity", index, identity, ok)
		}
	}
}
