package artifact

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestArtifactDigestUsesFramedFieldKinds(t *testing.T) {
	zero := identity.ContentID{}
	left := digest("artifact/test/digest", 1, bytesField(zero), uintField(1))
	right := digest("artifact/test/digest", 1, bytesField(zero), uintField(2))
	if !left.Available() || !right.Available() || left == right {
		t.Fatal("digest did not commit the scalar field value")
	}
	if got := digest("artifact/test/digest", 1, field{kind: 0}); got.Available() {
		t.Fatal("digest admitted an unknown field kind")
	}
}
