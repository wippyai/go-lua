package static

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/identity"
)

func TestTypeArgumentSequenceIdentityOwnsOrderAndCardinality(t *testing.T) {
	first, second := identity.ContentID{1}, identity.ContentID{2}
	empty, emptyOK := typeArgumentSequenceIdentity(nil)
	forward, forwardOK := typeArgumentSequenceIdentity([]identity.ContentID{first, second})
	replay, replayOK := typeArgumentSequenceIdentity([]identity.ContentID{first, second})
	reverse, reverseOK := typeArgumentSequenceIdentity([]identity.ContentID{second, first})
	if !emptyOK || empty.Count() != 0 || !forwardOK || !replayOK || !reverseOK {
		t.Fatal("lawful sequence identity was unavailable")
	}
	if !forward.Same(replay) || forward.Count() != 2 {
		t.Fatal("equal ordered sequences did not share one canonical receipt")
	}
	if forward.Same(reverse) {
		t.Fatal("sequence identity erased formal order")
	}
	if _, ok := typeArgumentSequenceIdentity([]identity.ContentID{{}}); ok {
		t.Fatal("sequence identity admitted an unavailable formal")
	}
}
