package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestFacts_Zero(t *testing.T) {
	f := Facts{}
	if f.FunctionFacts != nil {
		t.Error("zero Facts should have nil FunctionFacts")
	}
	if f.LiteralSigs != nil {
		t.Error("zero Facts should have nil LiteralSigs")
	}
	if f.CapturedTypes != nil {
		t.Error("zero Facts should have nil CapturedTypes")
	}
	if f.CapturedFields != nil {
		t.Error("zero Facts should have nil CapturedFields")
	}
}

func TestCapturedTypes_Basic(t *testing.T) {
	captured := make(CapturedTypes)
	sym := cfg.SymbolID(1)
	captured[sym] = product.FromType(typ.String)

	retrieved, ok := captured[sym]
	if !ok {
		t.Fatal("expected symbol to be in captured")
	}
	if !product.Equal(retrieved, product.FromType(typ.String)) {
		t.Error("expected string type")
	}
}

func TestCapturedFieldAssigns_Basic(t *testing.T) {
	assigns := make(CapturedFieldAssigns)
	nestedSym := cfg.SymbolID(1)
	capturedSym := cfg.SymbolID(2)
	foo := constraint.Segment{Kind: constraint.SegmentField, Name: "foo"}
	bar := constraint.Segment{Kind: constraint.SegmentField, Name: "bar"}

	assigns[nestedSym] = map[cfg.SymbolID]FieldValues{
		capturedSym: {
			foo: product.FromType(typ.String),
			bar: product.FromType(typ.Number),
		},
	}

	nestedAssigns, ok := assigns[nestedSym]
	if !ok {
		t.Fatal("expected nested symbol in assigns")
	}
	fields, ok := nestedAssigns[capturedSym]
	if !ok {
		t.Fatal("expected captured symbol in nested assigns")
	}
	if len(fields) != 2 {
		t.Errorf("expected 2 fields, got %d", len(fields))
	}
	if !product.Equal(fields[foo], product.FromType(typ.String)) {
		t.Error("expected foo to be string")
	}
}

func TestFacts_WithData(t *testing.T) {
	f := Facts{
		FunctionFacts: FunctionFacts{
			4: {
				Params:    product.LiftVector([]typ.Type{typ.Number}),
				Summary:   product.LiftVector([]typ.Type{typ.Boolean}),
				Narrow:    product.LiftVector([]typ.Type{typ.Boolean}),
				Signature: typ.Func().Returns(typ.Boolean).Build(),
			},
		},
	}

	if len(f.FunctionFacts) != 1 {
		t.Error("expected 1 function fact")
	}
}
