package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Intra-module (value,err) where value return is an AttrGet, consumed by if err==nil.
func TestZZAttrGetIntra(t *testing.T) {
	src := `
type User = {id: string, email: string}
local users: {[string]: User} = {}
local function get_email(id: string): (string?, string?)
    local u = users[id]
    if u then
        return u.email, nil
    end
    return nil, "not found"
end
local email, err = get_email("u1")
if err == nil then
    local e: string = email
end
`
	res := testutil.Check(src, testutil.WithStdlib(), testutil.WithCheckOption(check.WithCanonicalFlow()))
	for _, m := range testutil.ErrorMessages(res.Diagnostics) {
		t.Logf("DIAG-ATTRGET: %s", m)
	}
}
