package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
	"github.com/wippyai/go-lua/analysis/program/source"
)

func spanForTest() source.Span { return source.Span{File: "static.lua", StartLine: 1, StartCol: 1} }

func bindTypeForTest() bind.TypeDecl {
	return bind.TypeDecl{ID: 1, Kind: bind.TypeDeclAlias, Name: "T"}
}

func TestDeclaredCellTypeRequiresAuthoredType(t *testing.T) {
	var w Writer
	if err := w.DeclareCellType(1, nil, 2); err == nil {
		t.Fatal("DeclareCellType accepted a nil authored type")
	}
	if err := w.DeclareImplicitSelfType(1, spanForTest(), bindTypeForTest()); err == nil {
		t.Fatal("DeclareImplicitSelfType accepted an unavailable declaration")
	}
}
