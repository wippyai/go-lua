package front

import (
	"testing"

	pathdom "github.com/wippyai/go-lua/analysis/domain/path"
)

func TestBranchV1WireTranslatesCanonicalNamedRootAtBoundary(t *testing.T) {
	placeholder := pathdom.NewPlaceholder(0).Field("item")
	if got := placeholder.Key(); got != pathdom.PathKey("n2:$0.item") {
		t.Fatalf("in-memory key = %q, want canonical n2:$0.item", got)
	}
	if got := branchWirePathKey(placeholder); got != "$0.item" {
		t.Fatalf("v1 wire key = %q, want byte-compatible $0.item", got)
	}
}
