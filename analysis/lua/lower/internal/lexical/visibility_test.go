package lexical

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/bind"
)

func TestLexicalVisibilityRestoresShadowedIdentity(t *testing.T) {
	b := &Bodies{}
	id := bind.Symbol(7)
	b.install(id, 11)
	mark := len(b.undo)
	b.install(id, 19)
	if got, ok := b.Resolve(id); !ok || got != 19 || !b.Has(id) {
		t.Fatalf("shadowed visibility = %d/%t, want 19/true", got, ok)
	}
	b.restore(mark)
	if got, ok := b.Resolve(id); !ok || got != 11 {
		t.Fatalf("restored visibility = %d/%t, want 11/true", got, ok)
	}
}
