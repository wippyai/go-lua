package core

import (
	"testing"

	"github.com/wippyai/go-lua/types/typ"
)

func TestOperatorOnPendingOperandIsPending(t *testing.T) {
	if got := UnaryOp("#", typ.Unresolved); !typ.IsUnresolved(got) {
		t.Fatalf("#pending = %v, want unresolved", got)
	}
	if got := BinaryOp(typ.Unresolved, "+", typ.Integer); !typ.IsUnresolved(got) {
		t.Fatalf("pending + 1 = %v, want unresolved", got)
	}
	if got := BinaryOp(typ.String, "..", typ.Unresolved); !typ.IsUnresolved(got) {
		t.Fatalf("s .. pending = %v, want unresolved", got)
	}
}
