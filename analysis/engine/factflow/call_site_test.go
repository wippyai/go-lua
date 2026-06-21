package factflow

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/path"
	"github.com/wippyai/go-lua/analysis/symbol"
)

func TestCalleePathKeyMatchesCallSitePathKey(t *testing.T) {
	callee := path.NewPath(symbol.ID(7), "module").Field("run")
	want := callee.Key()
	key, ok := CalleePathKeyFromPath(callee)
	if !ok || key.PathKey() != want {
		t.Fatalf("CalleePathKeyFromPath = %q/%v, want %q/true", key.PathKey(), ok, want)
	}

	site := NewCallSite(CallSiteConfig{CalleePath: callee}).View()
	if got := site.CalleePathKey(); got != key {
		t.Fatalf("CallSiteView.CalleePathKey() = %q, want %q", got.PathKey(), key.PathKey())
	}

	if got, ok := CalleePathKeyFromPath(path.Path{}); ok || got.Valid() {
		t.Fatalf("empty CalleePathKeyFromPath = %q/%v, want invalid", got.PathKey(), ok)
	}
	if got, ok := CalleePathKeyFromPathKey(""); ok || got.Valid() {
		t.Fatalf("empty CalleePathKeyFromPathKey = %q/%v, want invalid", got.PathKey(), ok)
	}
}
