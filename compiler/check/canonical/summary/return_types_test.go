package summary

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/domain/paramevidence"
	"github.com/wippyai/go-lua/types/domain/value/product"
	"github.com/wippyai/go-lua/types/typ"
)

func TestReturnTypesProjectsSummaryReturns(t *testing.T) {
	got := ReturnTypes(Summary{
		Returns: []product.AbstractValue{
			product.FromType(typ.String),
			product.FromType(typ.Number),
		},
	})
	if len(got) != 2 {
		t.Fatalf("ReturnTypes len = %d, want 2", len(got))
	}
	if !typ.TypeEquals(got[0], typ.String) || !typ.TypeEquals(got[1], typ.Number) {
		t.Fatalf("ReturnTypes = [%v, %v], want [string, number]", got[0], got[1])
	}
	got[0] = typ.Boolean
	again := ReturnTypes(Summary{Returns: []product.AbstractValue{product.FromType(typ.String)}})
	if !typ.TypeEquals(again[0], typ.String) {
		t.Fatalf("ReturnTypes exposed mutable backing state: %v", again[0])
	}
}

func TestReturnTypesProjectsZeroSlotAsUnknown(t *testing.T) {
	got := ReturnTypes(Summary{Returns: []product.AbstractValue{{}}})
	if len(got) != 1 || !typ.IsUnknown(got[0]) {
		t.Fatalf("ReturnTypes zero slot = %#v, want [unknown]", got)
	}
}

func TestReturnValuesClonesAbstractTuple(t *testing.T) {
	stringAV := product.FromType(typ.String)
	numberAV := product.FromType(typ.Number)
	sum := Summary{Returns: []product.AbstractValue{stringAV, numberAV}}

	got := ReturnValues(sum)
	if len(got) != 2 || !product.Domain.Equal(got[0], stringAV) || !product.Domain.Equal(got[1], numberAV) {
		t.Fatalf("ReturnValues = %#v, want cloned abstract tuple", got)
	}
	got[0] = numberAV
	again := ReturnValues(sum)
	if !product.Domain.Equal(again[0], stringAV) {
		t.Fatalf("ReturnValues exposed mutable backing state: %#v", again)
	}
}

func TestFunctionSignatureWithSummaryReturns(t *testing.T) {
	sig := typ.Func().Param("x", typ.String).Returns(typ.Nil).Build()
	got := FunctionSignatureWithSummaryReturns(sig, Summary{
		Returns: []product.AbstractValue{product.FromType(typ.Number)},
	})
	if got == sig {
		t.Fatal("FunctionSignatureWithSummaryReturns returned original signature despite summary returns")
	}
	if len(got.Params) != 1 || !typ.TypeEquals(got.Params[0].Type, typ.String) {
		t.Fatalf("params not preserved: %#v", got.Params)
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Number) {
		t.Fatalf("returns = %#v, want [number]", got.Returns)
	}
	if unchanged := FunctionSignatureWithSummaryReturns(sig, Summary{}); unchanged != sig {
		t.Fatal("empty summary returns should preserve original signature pointer")
	}
}

func TestFunctionSignatureWithProjectedReturnsPreservesDeclaredReturnSignature(t *testing.T) {
	sig := typ.Func().Param("x", typ.String).Returns(typ.Boolean).Build()
	got := FunctionSignatureWithProjectedReturns(sig, true, Summary{
		Returns: []product.AbstractValue{product.FromType(typ.Number)},
	})
	if got != sig {
		t.Fatal("declared return signature should be preserved")
	}
}

func TestFunctionSignatureWithProjectedReturnsDoesNotRewriteParamsFromSummary(t *testing.T) {
	sig := typ.Func().OptParam("value", typ.Any).Build()
	got := FunctionSignatureWithProjectedReturns(sig, false, Summary{
		Params:  paramevidence.Contracts{0: paramevidence.DemandFromType(typ.String)},
		Returns: []product.AbstractValue{product.FromType(typ.Number)},
	})
	if got == nil || len(got.Params) != 1 {
		t.Fatalf("signature = %v, want one parameter", got)
	}
	if !got.Params[0].Optional || !typ.IsAny(got.Params[0].Type) {
		t.Fatalf("summary param demand leaked into signature param: %+v", got.Params[0])
	}
	if len(got.Returns) != 1 || !typ.TypeEquals(got.Returns[0], typ.Number) {
		t.Fatalf("returns = %#v, want [number]", got.Returns)
	}
}
