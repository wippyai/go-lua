package regression

import "testing"

func TestContextualBooleanDiscriminantRetainsValue(t *testing.T) {
	checkBothModes(t, `
type Result<T> = {ok: true, value: T} | {ok: false, error: string}
local function ok<T>(value: T): Result<T> return {ok = true, value = value} end
local function err<T>(message: string): Result<T> return {ok = false, error = message} end
return ok, err`, "")
}

func TestContextualBooleanDiscriminantRejectsWrongPayload(t *testing.T) {
	checkBothModes(t, `
type Result = {ok: true, value: string} | {ok: false, error: string}
local function bad(): Result return {ok = true, error = "bad"} end
return bad`, "cannot return")
}
