package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestLexicalBodyEntryRequiresCollector(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if _, err := b.Entry(source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("Entry accepted a missing collector")
	}
	if _, err := b.EnterFunction(source.Span{File: "lexical.lua"}, nil); err == nil {
		t.Fatal("EnterFunction accepted a nil function boundary")
	}
}

func TestLexicalFinishRejectsNoActiveBody(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if _, err := b.Finish(); err == nil {
		t.Fatal("Finish accepted an empty Body stack")
	}
}
