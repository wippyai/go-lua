package checktest

import "testing"

// A declared-optional local reassigned on one edge of a branch and then read
// at the join point, as a call argument, makes CallerOwnedParameterSource
// (analysis/check/body/call_argument_trust.go) walk into
// callerOwnedParameterDeclarationSource and
// factquery.dominatingDeclarationSource (analysis/engine/factquery/root_declaration.go)
// for a root declaration whose resolved source loops back onto the same
// (point, path). The cycle guard keyed only on ExprRef, so it never saw this
// declaration-source hop repeat and recursed until the process crashed with a
// stack overflow. A crash cannot be asserted like a normal test failure, so
// this test only needs to run to completion.
func TestCallerOwnedParameterSourceDoesNotOverflowOnDeclarationSourceCycle(t *testing.T) {
	result := Check(`
local ok = true
local x: string?
if ok then
    x = "v"
end
print(x)
`)
	defer result.ReleaseTransient()
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none for a sound declared-optional merge", result.Diagnostics)
	}
}
