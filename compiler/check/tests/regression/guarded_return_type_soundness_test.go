package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestGuardedReturnTypeRequiresEveryTruthyReturnToAgree(t *testing.T) {
	result := testutil.Check(`
local function choose(which: number)
    if which == 1 then return true, { category = "good" } end
    if which == 2 then return true, "bad" end
    return false, "excluded"
end
local include, info = choose(1)
if include then
    local category: string = info.category
end
`, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("a truthy return can carry a string, so the field read must fail")
	}
}
