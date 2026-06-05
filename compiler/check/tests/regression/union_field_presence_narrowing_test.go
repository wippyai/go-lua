package regression

import (
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

func TestUnionFieldPresenceElseNarrowsMissingFieldVariant(t *testing.T) {
	result := testutil.Check(`
type Accepted = {
	id: string,
	attempt: number,
}

type Rejected = {
	id: string,
	reason: string,
}

type Decision = Accepted | Rejected

local function decide(flag: boolean): Decision
	if flag then
		return {id = "job", attempt = 2}
	end
	return {id = "job", reason = "retry_limit"}
end

local outcome = decide(true)
if outcome.reason then
    local reason: string = outcome.reason
else
    local accepted_id: string = outcome.id
    local attempt: number = outcome.attempt
end
`)

	if result.HasError() {
		t.Fatalf("presence guard should narrow missing-field union variant: %v", testutil.ErrorMessages(result.Diagnostics))
	}
}
