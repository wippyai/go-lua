package flow

import (
	"testing"

	"github.com/wippyai/go-lua/types/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/lattice"
	"github.com/wippyai/go-lua/types/typ"
)

func TestIndexWriteAdmissionFactsDomain_Laws(t *testing.T) {
	lattice.LawSuite[IndexWriteAdmissionFacts]{
		Name:   "IndexWriteAdmissionFacts",
		Domain: IndexWriteAdmissionFactsDomain,
		Sample: indexWriteAdmissionFactsSample(),
		Format: IndexWriteAdmissionFacts.Format,
	}.Run(t)
}

func TestIndexWriteAdmissionFactsJoinKeepsOnlyCommonProofs(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(1), "m")
	key := constraint.NewPath(cfg.SymbolID(2), "k")
	valuePath := constraint.NewPath(cfg.SymbolID(3), "v")
	otherValue := constraint.NewPath(cfg.SymbolID(4), "w")
	common := IndexWriteAdmissionFact{
		Target:    IndexWriteAdmissionPathKey(target),
		KeyPath:   IndexWriteAdmissionPathKey(key),
		Key:       product.FromType(typ.String),
		ValuePath: IndexWriteAdmissionPathKey(valuePath),
		Value:     product.FromType(typ.String),
	}
	left := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
		common,
		{
			Target:    IndexWriteAdmissionPathKey(target.Field("items")),
			KeyPath:   IndexWriteAdmissionPathKey(key),
			Key:       product.FromType(typ.String),
			ValuePath: IndexWriteAdmissionPathKey(otherValue),
			Value:     product.FromType(typ.Boolean),
		},
	})
	right := IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
		{
			Target:    common.Target,
			KeyPath:   common.KeyPath,
			Key:       product.FromType(typ.String),
			ValuePath: common.ValuePath,
			Value:     product.FromType(typ.Number),
		},
	})

	joined := IndexWriteAdmissionFactsDomain.Join(left, right)
	got, ok := joined.Admission(IndexWriteQuery{
		Target:    target,
		KeySymbol: key.Symbol,
		KeyType:   typ.String,
		ValuePath: valuePath,
	})
	if !ok {
		t.Fatal("join dropped common admission proof")
	}
	want := typ.NewUnion(typ.String, typ.Number)
	if !typ.TypeEquals(got.ProjectValue(), want) {
		t.Fatalf("joined admission value = %v, want %v", got.ProjectValue(), want)
	}
	if _, ok := joined.Admission(IndexWriteQuery{
		Target:    target.Field("items"),
		KeySymbol: key.Symbol,
		KeyType:   typ.String,
		ValuePath: otherValue,
	}); ok {
		t.Fatalf("join kept one-branch admission proof: %s", joined.Format())
	}
}

func TestIndexWriteAdmissionFactsMatchesByKeyValueWhenKeyPathAbsent(t *testing.T) {
	target := constraint.NewPath(cfg.SymbolID(10), "m")
	facts := IndexWriteAdmissionFacts{}.With(IndexWriteAdmissionFact{
		Target: IndexWriteAdmissionPathKey(target),
		Key:    product.FromType(typ.LiteralString("name")),
		Value:  product.FromType(typ.String),
	})

	if _, ok := facts.Admission(IndexWriteQuery{Target: target, KeyType: typ.LiteralString("name")}); !ok {
		t.Fatal("literal-key query did not match literal-key admission proof")
	}
	if _, ok := facts.Admission(IndexWriteQuery{Target: target, KeyType: typ.LiteralString("other")}); ok {
		t.Fatal("literal-key query matched incompatible key proof")
	}
}

func indexWriteAdmissionFactsSample() []IndexWriteAdmissionFacts {
	target := SymbolPathKey(cfg.SymbolID(1), nil)
	key := SymbolPathKey(cfg.SymbolID(2), nil)
	valuePath := SymbolPathKey(cfg.SymbolID(3), nil)
	otherTarget := SymbolPathKey(cfg.SymbolID(4), []constraint.Segment{{Kind: constraint.SegmentField, Name: "items"}})
	return []IndexWriteAdmissionFacts{
		IndexWriteAdmissionFactsDomain.Bottom(),
		IndexWriteAdmissionFactsDomain.Top(),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{{
			Target:    target,
			KeyPath:   key,
			Key:       product.FromType(typ.String),
			ValuePath: valuePath,
			Value:     product.FromType(typ.Number),
		}}),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{{
			Target: target,
			Key:    product.FromType(typ.LiteralString("name")),
			Value:  product.FromType(typ.String),
		}}),
		IndexWriteAdmissionFactsOf([]IndexWriteAdmissionFact{
			{
				Target:    target,
				KeyPath:   key,
				Key:       product.FromType(typ.String),
				ValuePath: valuePath,
				Value:     product.FromType(typ.Integer),
			},
			{
				Target: otherTarget,
				Key:    product.FromType(typ.Number),
				Value:  product.FromType(typ.Boolean),
			},
		}),
	}
}
