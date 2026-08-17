package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
)

func TestFunctionRunRequiresPrivateContinuation(t *testing.T) {
	w := Writer{stack: &continuation.Stack{}}
	if err := w.Run(); err == nil {
		t.Fatal("Run accepted an empty function continuation")
	}
}

func TestFunctionStepNextAdvancesCursorWithoutChangingOwner(t *testing.T) {
	current := step{kind: stepHeaderFormal, index: 2, owner: 9}
	next := current.next()
	if next.index != 3 || next.kind != current.kind || next.owner != current.owner {
		t.Fatalf("next step = %#v, want cursor 3 with unchanged kind/owner", next)
	}
}
