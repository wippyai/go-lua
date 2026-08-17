package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestLexicalFunctionConstructionRequiresNestedBody(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if _, err := b.DeclareFunction(source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("DeclareFunction accepted an empty Body stack")
	}
	if err := b.FillFunction(1, 0, 0, -1); err == nil {
		t.Fatal("FillFunction accepted a function without an active nested Body")
	}
}
