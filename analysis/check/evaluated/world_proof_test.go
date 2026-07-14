package evaluated

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/value/product"
	"github.com/wippyai/go-lua/analysis/domain/value/typevalue"
	"github.com/wippyai/go-lua/analysis/test/value/standard"
)

func TestWorldProofAcceptsOwnedScalarAndRejectsPointerLiteralConstant(t *testing.T) {
	reg := standard.Registry()
	owned := WorldProof{Expressions: []Expression{{
		ID: 1, Op: ExpressionConstant, Constant: product.Top(),
		Scalar: Scalar{Kind: ScalarString, String: "string"},
	}}}
	if err := validateWorldProof(reg, owned); err != nil {
		t.Fatalf("owned scalar DTO rejected: %v", err)
	}
	pointerBacked := WorldProof{Expressions: []Expression{{
		ID: 1, Op: ExpressionConstant, Constant: typevalue.LiteralString(reg, "string"),
	}}}
	if err := validateWorldProof(reg, pointerBacked); err == nil {
		t.Fatal("pointer-backed literal product crossed evaluated proof boundary")
	}
}

func TestWorldProofRejectsPayloadInUnusedExpressionField(t *testing.T) {
	reg := standard.Registry()
	proof := WorldProof{Expressions: []Expression{{
		ID: 1, Op: ExpressionRoot, RootKind: RootParam,
		Constant: typevalue.LiteralString(reg, "hidden"),
	}}}
	if err := validateWorldProof(reg, proof); err == nil {
		t.Fatal("root expression retained hidden pointer-backed constant")
	}
}
