package api

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/cfg"
	"github.com/wippyai/go-lua/types/constraint"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

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
