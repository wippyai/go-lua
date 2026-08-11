package bind

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/ast"
)

func TestDirectGlobalCallsEnumeratePlainGlobalHeadsInSourceOrder(t *testing.T) {
	_, result := parseBindSource(t, `
require("first")
other()
require(require("nested"))
local require = function() end
require("shadowed")
module.require("member")
`)

	calls := result.DirectGlobalCalls()
	if got, want := len(calls), 4; got != want {
		t.Fatalf("DirectGlobalCalls length = %d, want %d", got, want)
	}
	for index, want := range []string{"require", "other", "require", "require"} {
		ident, ok := calls[index].Call.Func.(*ast.IdentExpr)
		if !ok || ident == nil || ident.Value != want {
			t.Fatalf("DirectGlobalCalls[%d] head = %T/%v, want %q", index, calls[index].Call.Func, ident, want)
		}
		if !calls[index].Global.Matches(want) {
			t.Fatalf("DirectGlobalCalls[%d] global does not match %q", index, want)
		}
	}

	// The returned slice is detached; consumers cannot alter binder evidence.
	calls[0].Call = nil
	if fresh := result.DirectGlobalCalls(); len(fresh) != 4 || fresh[0].Call == nil {
		t.Fatalf("DirectGlobalCalls did not return detached evidence: %#v", fresh)
	}
}
