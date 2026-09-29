package narrow

import (
	"testing"

	"github.com/wippyai/go-lua/types/kind"
	"github.com/wippyai/go-lua/types/typ"
)

func TestNarrowingKeepsPendingValuePending(t *testing.T) {
	if got := FilterByKind(typ.Unresolved, kind.Record); !typ.IsUnresolved(got) {
		t.Fatalf("kind test on pending value = %v, want unresolved", got)
	}
	got := FilterByKind(typ.NewUnion(typ.Unresolved, typ.String), kind.String)
	if u, ok := got.(*typ.Union); !ok || !u.Contains(typ.Unresolved) || !u.Contains(typ.String) {
		t.Fatalf("kind test on pending alternative = %v, want string | unresolved", got)
	}
	if got := ToFalsy(typ.Unresolved); !typ.IsUnresolved(got) {
		t.Fatalf("falsy part of pending value = %v, want unresolved", got)
	}
}
