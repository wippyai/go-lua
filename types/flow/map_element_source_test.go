package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/typ"
)

func TestMapElementTypeAtOpenRecordDynamicRead(t *testing.T) {
	c := cfg.New()
	g := newMockSSAGraph(c)
	sym := setupSymbol(g, "messages", []cfg.Point{c.Entry()})
	setVersion(g, c.Entry(), sym, cfg.Version{Root: "messages", Symbol: sym, ID: 1})
	inputs := newInputs(g)
	inputs.Decomposer = testMapElementDecomposer{}
	inputs.DeclaredTypes[sym] = typ.NewRecord().SetOpen(true).SetComplete(true).Build()
	s := Solve(inputs, testResolver())
	got := s.mapElementTypeAt(c.Entry(), &MapElementSource{MapPath: constraint.Path{Root: "messages", Symbol: sym}})
	if !typ.IsUnknown(got) {
		t.Fatalf("got %v, want unknown for unlisted open-table value", got)
	}
}
