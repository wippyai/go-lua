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

func TestMapElementTypeAtUsesIndexPresence(t *testing.T) {
	value := typ.NewMap(typ.String, typ.Unknown)
	for _, container := range []typ.Type{
		typ.NewMap(typ.String, value),
		typ.NewUnion(
			typ.NewRecord().MapComponent(typ.String, value).SetComplete(true).Build(),
			typ.NewRecord().SetComplete(true).Build(),
		),
	} {
		c := cfg.New()
		g := newMockSSAGraph(c)
		sym := setupSymbol(g, "objects", []cfg.Point{c.Entry()})
		key := setupSymbol(g, "k", []cfg.Point{c.Entry()})
		setVersion(g, c.Entry(), sym, cfg.Version{Root: "objects", Symbol: sym, ID: 1})
		setVersion(g, c.Entry(), key, cfg.Version{Root: "k", Symbol: key, ID: 1})
		inputs := newInputs(g)
		inputs.Decomposer = testMapElementDecomposer{}
		inputs.DeclaredTypes[sym] = container
		inputs.DeclaredTypes[key] = typ.String
		s := Solve(inputs, testResolver())
		got := s.mapElementTypeAt(c.Entry(), &MapElementSource{
			MapPath: constraint.Path{Root: "objects", Symbol: sym}, KeySymbol: key, KeyVar: "k",
		})
		if want := typ.NewOptional(value); !typ.TypeEquals(got, want) {
			t.Fatalf("dynamic read from %v = %v, want %v", container, got, want)
		}
	}
}
