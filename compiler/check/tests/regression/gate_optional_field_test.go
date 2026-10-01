package regression

import "testing"

func TestGateNilableFieldFitsOptionalSlot(t *testing.T) {
	checkBothModes(t, `
type Options = {body?: string, headers?: {[string]: string}}
local function request(opts: Options) end
local function send(body: string?)
 local opts: {body: string?, headers: {[string]: string}} = {body = body, headers = {}}
 request(opts)
end
return send`, "")
}

func TestGateOptionalFieldRejectsWrongPresentValue(t *testing.T) {
	checkBothModes(t, `
local function request(opts: {body?: string}) end
local function send(body: number?)
 local opts: {body: number?} = {body = body}
 request(opts)
end
return send`, "argument 1: expected {body?: string}")
}

func TestGateNilableFieldDoesNotFitRequiredSlot(t *testing.T) {
	checkBothModes(t, `
local function request(opts: {body: string}) end
local function send(body: string?)
 local opts: {body: string?} = {body = body}
 request(opts)
end
return send`, "argument 1: expected {body: string}")
}
