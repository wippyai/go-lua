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

func TestSequentialArtifactDigestsMatchIndependentWriters(t *testing.T) {
	zero := identity.ContentID{}
	wantLeft := digest("artifact/test/digest", 1, bytesField(zero), uintField(1))
	wantRight := digest("artifact/test/digest", 1, bytesField(zero), uintField(2))
	if !wantLeft.Available() || !wantRight.Available() || wantLeft == wantRight {
		t.Fatal("independent digests were not distinct")
	}
	for index := 0; index < 8; index++ {
		gotLeft := digest("artifact/test/digest", 1, bytesField(zero), uintField(1))
		gotRight := digest("artifact/test/digest", 1, bytesField(zero), uintField(2))
		if gotLeft != wantLeft {
			t.Fatalf("left digest %d diverged from the independent writer", index)
		}
		if gotRight != wantRight {
			t.Fatalf("right digest %d diverged from the independent writer", index)
		}
	}
}
