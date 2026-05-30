package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// zzGradAnyProbe traces the gradual-any argument compatibility long-tail: an
// unannotated parameter (and a field read off one) is gradual `any`; passing it
// to a parameter typed string must be accepted.
func TestZZGradAnyProbe(t *testing.T) {
	cases := map[string]string{
		"field-of-unannotated-param": `
local function decode(s: string): any
    return {}
end
local function parse(resp)
    return decode(resp.body)
end
return parse
`,
		"resolved-callee-unannotated-arg": `
local json = {}
function json.decode(source: string): (any, string?)
    return {}, nil
end
local function parse(http_response)
    return json.decode(http_response.body)
end
return parse
`,
		"assign-field-any-to-string": `
local function get(page)
    local name: string = page.data_func
    return name
end
return get
`,
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
			for _, m := range testutil.ErrorMessages(res.Diagnostics) {
				t.Logf("CANON DIAG: %s", m)
			}
			res2 := testutil.Check(src, testutil.WithStdlib())
			for _, m := range testutil.ErrorMessages(res2.Diagnostics) {
				t.Logf("LEGACY DIAG: %s", m)
			}
		})
	}
}
