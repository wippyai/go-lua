package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestRegression_BroadMapReturnKeepsProvenFields pins the re-applied
// broad-map return rule: a declared broad Map return keeps the inferred body
// record's proven field types, and table values flowing through and/or keep
// their shape so indexing them stays valid.
func TestRegression_BroadMapReturnKeepsProvenFields(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{
			Name: "single record body recovers field type",
			Code: `type Map = { [string]: any }
local function row(): Map
    return { binding_id = tostring(1) }
end
local binding = row()
local s: string = binding.binding_id`,
			Stdlib: true,
		},
		{
			Name: "table value through and or keeps indexable shape",
			Code: `local function pick(c: boolean)
    local t = { "x" }
    local a = (c and t) or { "dflt" }
    return { e = a }
end
local r = pick(true)
print(r.e[1])`,
			Stdlib: true,
		},
	})
}
