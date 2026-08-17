package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestStaticDeclarationHostRequiresBoundIdentity(t *testing.T) {
	var w Writer
	if _, ok := w.Host(bind.TypeDecl{ID: 1, Kind: bind.TypeDeclAlias}); ok {
		t.Fatal("Host returned an identity from an empty declaration table")
	}
	if err := w.Predeclare(1, nil); err == nil {
		t.Fatal("Predeclare accepted an unavailable binder")
	}
	if _, err := w.BeginAlias(nil); err == nil {
		t.Fatal("BeginAlias accepted a missing definition")
	}
}
