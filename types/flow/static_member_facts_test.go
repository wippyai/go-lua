package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestStaticMemberFactsDomain_Laws(t *testing.T) {
	lattice.LawSuite[StaticMemberFacts]{
		Name:   "StaticMemberFacts",
		Domain: StaticMemberFactsDomain,
		Sample: staticMemberFactsSample(),
		Format: StaticMemberFacts.Format,
	}.Run(t)
}

func TestStaticMemberFactsJoinKeepsCommonProvenPaths(t *testing.T) {
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "kind"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}})
	pInt := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}})

	left := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.String)},
		{Path: pIndex, Value: product.FromType(typ.Number)},
	})
	right := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.Integer)},
		{Path: pInt, Value: product.FromType(typ.Boolean)},
	})

	joined := StaticMemberFactsDomain.Join(left, right)
	if _, ok := joined.Value(pIndex); ok {
		t.Fatal("join kept string-index fact not proven by both predecessors")
	}
	if _, ok := joined.Value(pInt); ok {
		t.Fatal("join kept int-index fact not proven by both predecessors")
	}
	got, ok := joined.Value(pField)
	if !ok {
		t.Fatal("join dropped common field fact")
	}
	want := product.Domain.Join(product.FromType(typ.String), product.FromType(typ.Integer))
	if !product.Domain.Equal(got, want) {
		t.Fatalf("joined field value = %s, want %s", got.ProjectValue(), want.ProjectValue())
	}
}

func TestStaticMemberFactsKillSubtreeUsesStructuralPathPrefix(t *testing.T) {
	root := SymbolPathKey(cfg.SymbolID(1), nil)
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})
	other := SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})

	facts := StaticMemberFactsOf([]StaticMemberFact{
		{Path: pField, Value: product.FromType(typ.String)},
		{Path: pIndex, Value: product.FromType(typ.Number)},
		{Path: other, Value: product.FromType(typ.Boolean)},
	})

	killed := facts.KillSubtree(root)
	if _, ok := killed.Value(pField); ok {
		t.Fatal("root kill kept dot-field fact")
	}
	if _, ok := killed.Value(pIndex); ok {
		t.Fatal("root kill kept string-index fact")
	}
	if _, ok := killed.Value(other); !ok {
		t.Fatal("root kill removed unrelated symbol fact")
	}
}

func staticMemberFactsSample() []StaticMemberFacts {
	pField := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentField, Name: "foo"}})
	pIndex := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: "foo"}})
	pEmpty := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexString, Name: ""}})
	pInt := SymbolPathKey(cfg.SymbolID(1), []constraint.Segment{{Kind: constraint.SegmentIndexInt, Index: 1}})
	pOther := SymbolPathKey(cfg.SymbolID(2), []constraint.Segment{{Kind: constraint.SegmentField, Name: "bar"}})

	return []StaticMemberFacts{
		StaticMemberFactsDomain.Bottom(),
		StaticMemberFactsDomain.Top(),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pField, Value: product.FromType(typ.String)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pIndex, Value: product.FromType(typ.Number)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pEmpty, Value: product.FromType(typ.Boolean)}}),
		StaticMemberFactsOf([]StaticMemberFact{{Path: pInt, Value: product.FromType(typ.Integer)}}),
		StaticMemberFactsOf([]StaticMemberFact{
			{Path: pField, Value: product.FromType(typ.String)},
			{Path: pOther, Value: product.FromType(typ.Boolean)},
		}),
		StaticMemberFactsOf([]StaticMemberFact{
			{Path: pField, Value: product.FromType(typ.Number)},
			{Path: pIndex, Value: product.FromType(typ.String)},
			{Path: pInt, Value: product.FromType(typ.Boolean)},
		}),
	}
}
