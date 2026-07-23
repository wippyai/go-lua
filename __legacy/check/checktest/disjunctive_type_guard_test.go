package checktest

import (
	"testing"

	"github.com/wippyai/go-lua/analysis/check/diagnostics"
)

// TestDisjunctiveTypeGuardNarrowsReceiverAcrossBothArms pins the real-world
// shape audited as bucket 2 of the "optional method call" false-positive
// family: a type() guard split across an `or`, where every disjunct excludes
// nil. Each arm alone narrows err's type; the guard as a whole must prove err
// non-nil on the true edge (err : userdata|table), so the method call needs no
// separate nil check.
func TestDisjunctiveTypeGuardNarrowsReceiverAcrossBothArms(t *testing.T) {
	result := Check(`
type Err = {
    kind: (self: Err) -> string,
}

local function handle(err: Err?)
    if type(err) == "userdata" or type(err) == "table" then
        err:kind()
    end
end
`)
	for _, d := range result.Diagnostics {
		if d.Code == diagnostics.CodeOptionalMethodCall {
			t.Fatalf(
				"got optional-method-call diagnostic despite type(err) == \"userdata\" or type(err) == \"table\" proving err non-nil on the true edge: %#v",
				d,
			)
		}
	}
}
