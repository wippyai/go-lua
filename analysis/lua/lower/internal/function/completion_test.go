package function

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/lua/lower/internal/continuation"
)

func TestFunctionPrivateContinuationStackIsLIFO(t *testing.T) {
	w := Writer{}
	w.push(step{kind: stepBegin})
	w.push(step{kind: stepCloseBody})

	if got := w.pop(); got.kind != stepCloseBody {
		t.Fatalf("pop returned %d, want close-body step", got.kind)
	}
	if got := w.pop(); got.kind != stepBegin {
		t.Fatalf("second pop returned %d, want begin step", got.kind)
	}
	if !w.Clean() {
		t.Fatal("private continuation stack retained completed work")
	}
}

func TestFunctionCompletionRejectsMissingIdentity(t *testing.T) {
	w := Writer{}
	if err := w.complete(0, completion{}, &continuation.Stack{}); err == nil {
		t.Fatal("complete accepted a missing function identity")
	}
}
