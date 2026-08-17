package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestLexicalVarargRequiresActiveFunctionEvidence(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if _, err := b.Vararg(source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("Vararg accepted a request outside a Body")
	}
	if err := b.ScheduleVararg(nil, 1, source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("ScheduleVararg accepted a missing expression")
	}
}
