package functionfact

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
	"github.com/wippyai/go-lua/compiler/check/api"
	"github.com/wippyai/go-lua/types/typ"
)

func TestSolvedSignatureFromResult_UsesSourceSignatureWithoutNarrowSynth(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	source := typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()

	got := SolvedSignatureFromResult(&api.FuncResult{SourceSignature: source}, fn)
	if !typ.TypeEquals(got, source) {
		t.Fatalf("SolvedSignatureFromResult() = %v, want %v", got, source)
	}
}

func TestSolvedSignatureFromView_UsesSourceSignatureWithoutNarrowSynth(t *testing.T) {
	fn := &ast.FunctionExpr{ParList: &ast.ParList{}}
	source := typ.Func().Param("value", typ.String).Returns(typ.Boolean).Build()

	got := SolvedSignatureFromView(&api.FuncAnalysisView{SourceSignature: source}, fn)
	if !typ.TypeEquals(got, source) {
		t.Fatalf("SolvedSignatureFromView() = %v, want %v", got, source)
	}
}
