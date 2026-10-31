package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// TestRegression_ArrayIndexWithAnyKeyStaysGradual pins that indexing a
// declared array with an any-typed key stays gradual: the key may be a valid
// integer at runtime, and indexing a table never throws, so the read yields
// an optional element instead of a diagnostic.
func TestRegression_ArrayIndexWithAnyKeyStaysGradual(t *testing.T) {
	testutil.RunCases(t, []testutil.Case{
		{
			Name: "annotated array param indexed by any key",
			Code: `local function accept(batch: { any }, judgments: any): { any }
    local ranked: { any } = {}
    for _, raw in ipairs(judgments :: { any }) do
        local judgment = raw :: any
        local pair = batch[judgment.pair]
        if pair ~= nil then ranked[#ranked + 1] = pair end
    end
    return ranked
end`,
			Stdlib: true,
		},
	})
}
