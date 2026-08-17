package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func TestLexicalCellAdmissionRequiresLiveAuthority(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if b.CellMark() != 0 || b.CaptureMark() != 0 {
		t.Fatal("new lexical cell ranges were not empty")
	}
	if _, err := b.Declare(bind.Symbol(0), source.Span{File: "lexical.lua"}); err == nil {
		t.Fatal("Declare accepted a zero binder symbol")
	}
}
