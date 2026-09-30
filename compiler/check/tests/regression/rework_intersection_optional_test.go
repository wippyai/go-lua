package regression

import "testing"

func TestIntersectionFitsItsOptionalDomain(t *testing.T) {
	checkBothModes(t, `
type State = {[string]: any} & {rollback: {string}?}
local function copy(value: State): State? return value end
return copy`, "")
}

func TestIntersectionOptionalDoesNotDropObligation(t *testing.T) {
	checkBothModes(t, `
type State = {[string]: any} & {id: string}
local function copy(value: {[string]: any}): State? return value end
return copy`, "cannot return")
}
