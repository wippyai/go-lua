package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check"
	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

const predicateReturnPrelude = `
type Request = {workspace_id: string}
type Object = {[string]: unknown}
local function coin(): boolean return math.random() > 0.5 end
`

func predicateReturnSource(stringArm, requestArm, use string) string {
	return predicateReturnPrelude + `
local function matches(expected: string | Request, value: Object): boolean
    if type(expected) == "string" then return ` + stringArm + ` end
    return ` + requestArm + `
end
local function reply(value: Object, expected: string | Request): string?
    if not matches(expected, value) then return nil end
    ` + use + `
    return nil
end
return reply
`
}

func TestPredicateTruthyReturnKeepsEveryReturningArm(t *testing.T) {
	sources := []string{
		predicateReturnPrelude + `
local function workspace(value: unknown, expected: string): string?
    if type(value) ~= "string" or value ~= expected then return nil end
    return value
end
local function expected_identity(expected: string | Request, value: Object): boolean
    if type(expected) == "string" then return workspace(value.workspace_id, expected) ~= nil end
    return value.workspace_id == expected.workspace_id
end
local function reply(value: Object, expected: string | Request): string?
    if not expected_identity(expected, value) then return nil end
    local workspace_id: string
    if type(expected) == "string" then workspace_id = expected else workspace_id = (expected :: Request).workspace_id end
    return workspace_id
end
return reply
`,
		predicateReturnSource(`true`, `expected.workspace_id == "x"`, `if type(expected) == "string" then local s: string = expected end`),
		predicateReturnSource(`coin()`, `expected.workspace_id == "x"`, `if type(expected) == "string" then local s: string = expected end`),
		predicateReturnSource(`value.flag ~= nil`, `expected.workspace_id == "x"`, `if type(expected) == "string" then local s: string = expected end`),
		// returning from the predicate at all implies neither arm
		predicateReturnPrelude + `
local function matches(expected: string | Request): boolean
    if type(expected) == "string" then return true end
    return expected.workspace_id == "x"
end
local function check(expected: string | Request): boolean
    local ok = matches(expected)
    if type(expected) ~= "string" then return expected.workspace_id == "y" end
    return ok
end
return check
`,
	}
	for _, source := range sources {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			if result.HasError() {
				t.Errorf("strict=%v source=%s: %v", strict, source, testutil.ErrorMessages(result.Errors))
			}
		}
	}
}

func TestPredicateTruthyReturnNarrowsOnlyByReturningArms(t *testing.T) {
	for _, tt := range []struct {
		source string
		want   string
	}{
		// the string arm returns false, so a truthy result excludes string
		{predicateReturnSource(`false`, `expected.workspace_id == "x"`, `local s: string = expected`), "cannot assign"},
		// the string arm can return truthy, so a truthy result keeps string
		{predicateReturnSource(`coin()`, `expected.workspace_id == "x"`, `local r: Request = expected`), "cannot assign"},
		{predicateReturnSource(`true`, `expected.workspace_id == "x"`, `local r: Request = expected`), "cannot assign"},
	} {
		for _, strict := range []bool{false, true} {
			result := testutil.Check(tt.source, testutil.WithStdlib(), testutil.WithCheckOptions(check.Options{Strict: strict}))
			messages := strings.Join(testutil.ErrorMessages(result.Errors), "; ")
			if !strings.Contains(messages, tt.want) {
				t.Errorf("strict=%v expected %q for %s, got %q", strict, tt.want, tt.source, messages)
			}
		}
	}
}
