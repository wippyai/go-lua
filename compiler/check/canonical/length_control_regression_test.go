package canonical_test

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestCanonicalFreshEmptyReturnedMemberKillsNumericForBody(t *testing.T) {
	res := testutil.Check(`
local T = {}
local mt = { __index = T }

function T.new()
    return setmetatable({
        xs = table.create(16, 0),
    }, mt)
end

function T:f()
    for i = 1, #self.xs - 1 do
        local current = self.xs[i]
        local next = self.xs[i + 1]
        return current.id .. next.id
    end
    return ""
end

return T.new():f()
`, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("diagnostics: %v", msgs)
	}
}

func TestCanonicalUnionOfFreshEmptyReturnsKillsNumericForBody(t *testing.T) {
	res := testutil.Check(`
local function make(flag: boolean)
    if flag then
        return table.create(16, 0)
    end
    return table.create(4, 0)
end

local function run(flag: boolean)
    local xs = make(flag)
    for i = 1, #xs - 1 do
        local current = xs[i]
        return current.id
    end
    return ""
end

return run
`, testutil.WithStdlib())
	if msgs := testutil.ErrorMessages(res.Diagnostics); len(msgs) != 0 {
		t.Fatalf("diagnostics: %v", msgs)
	}
}
