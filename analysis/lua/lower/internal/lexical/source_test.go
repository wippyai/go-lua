package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestLexicalSourceEvidenceRejectsZeroTerms(t *testing.T) {
	b := New(nil, nil, nil, "lexical.lua", nil, nil, nil)
	if err := b.Append(0); err == nil {
		t.Fatal("Append accepted a zero source term")
	}
	if _, err := b.ReserveCell(bind.Symbol(0)); err == nil {
		t.Fatal("ReserveCell accepted a zero binder symbol")
	}
	if _, err := b.ResolveCell(CellEvidence{}); err == nil {
		t.Fatal("ResolveCell accepted empty source evidence")
	}
}
