package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// A fluent builder and the instance it opens share stable recursion snapshots.
func TestCallbackCaptureBuilderConverges(t *testing.T) {
	source := `local mod = {}
mod.contract = {get = function()
 local inst = {resolve = function(_self: any, args: any) return {} end}
 local builder = {}
 builder.next = function(_self: any, arg: any) return builder end
 builder.open = function() return inst, nil end
 return builder, nil
end}
return mod
`
	for _, strict := range []bool{false, true} {
		name := "gradual"
		if strict {
			name = "strict"
		}
		t.Run(name, func(t *testing.T) {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if len(result.Diagnostics) != 0 {
				t.Fatalf("unexpected diagnostics: %v", result.Diagnostics)
			}
		})
	}
}
