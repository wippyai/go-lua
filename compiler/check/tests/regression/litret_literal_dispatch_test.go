package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestLitretLiteralParamMakesNilReturnDead(t *testing.T) {
	source := `
local http = { response = function(): string return "ok" end }
local function mod(name: "http")
    if name == "http" then return http end
    return nil
end
local h = mod("http")
local res: string = h.response()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		for _, e := range result.Errors {
			t.Logf("error: %s at %d:%d", e.Message, e.Position.Line, e.Position.Column)
		}
		t.Fatal("expected no errors when param is literal \"http\" and nil return is dead")
	}
}
