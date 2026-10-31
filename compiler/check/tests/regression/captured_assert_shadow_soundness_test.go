package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCapturedAssertShadowMayRebindLocal(t *testing.T) {
	result := testutil.Check(`
local seen_payload: { model: string }? = { model = "before" }
local function assert(v: any): any
    if v == nil then error("missing") end
    seen_payload = nil
    return v
end
assert(seen_payload)
local model: string = seen_payload.model
`, testutil.WithStdlib())
	if !result.HasError() {
		t.Fatal("expected an error after shadowed assert rebinds captured local")
	}
}
