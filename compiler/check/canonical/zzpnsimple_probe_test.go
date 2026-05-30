package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestZZParamNarrowSimple(t *testing.T) {
	// not_nil: errors if val IS nil -> proves val non-nil after
	cases := map[string]string{
		"not_nil-param": `
local function not_nil(val: any)
    if val == nil then
        error("nil")
    end
end
local function f(x: string?): string
    not_nil(x)
    return x
end
return f
`,
		"is_nil-param": `
local function is_nil(val: any)
    if val ~= nil then
        error("expected nil")
    end
end
local function f(x: string?): nil
    is_nil(x)
    return x
end
return f
`,
		"is_nil-msg-param": `
local function is_nil(val: any, msg: string?)
    if val ~= nil then
        error(msg or "expected nil", 2)
    end
end
local function f(x: string?): nil
    is_nil(x, "m")
    return x
end
return f
`,
	}
	for name, src := range cases {
		res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
		msgs := testutil.ErrorMessages(res.Diagnostics)
		t.Logf("%s: %d diags", name, len(msgs))
		for _, m := range msgs {
			t.Logf("    %s", m)
		}
	}
}
