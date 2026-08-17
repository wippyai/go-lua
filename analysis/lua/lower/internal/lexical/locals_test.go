package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestLexicalLocalSchedulingRequiresBinderAndStatement(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if err := b.ScheduleLocal(nil, 1, source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("ScheduleLocal accepted a missing local statement")
	}
	if err := b.Run(); err == nil {
		t.Fatal("Run accepted an empty local continuation")
	}
	if _, ok := b.reservedCell(-1, 0); ok {
		t.Fatal("reservedCell returned a value for a negative mark")
	}
}
