package structure_test

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/domain/composite"
)

func TestZZDigestProbe(t *testing.T) {
	sealed, failure := composite.Table()
	if failure.Available() || sealed == nil {
		t.Fatalf("rejected: %d %d", failure.Contributor, failure.Law)
	}
	t.Logf("TABLE DIGEST %x", sealed.Digest())
}
