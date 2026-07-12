package program

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/fixpoint/ref"
	"github.com/wippyai/go-lua/analysis/check/fixpoint/summary"
	"github.com/wippyai/go-lua/analysis/engine/factflow"
)

func TestParamInferenceCallSiteIdentityIncludesLexicalOwner(t *testing.T) {
	inferred := newParamInference(nil, nil)
	expr := factflow.ExprRef(1)
	left := summary.DefaultSummaryKey(ref.FuncRef{ID: 1})
	right := summary.DefaultSummaryKey(ref.FuncRef{ID: 2})

	if !inferred.markObserved(left, expr) {
		t.Fatal("first lexical call site was not observed")
	}
	if !inferred.markObserved(right, expr) {
		t.Fatal("same body-local ExprRef in another lexical owner was incorrectly deduplicated")
	}
	if inferred.markObserved(left, expr) {
		t.Fatal("same lexical call site was observed twice")
	}

	context := left
	context.Entry.Values = 1
	if inferred.markObserved(context, expr) {
		t.Fatal("context variant of the same lexical call site was observed twice")
	}
}
