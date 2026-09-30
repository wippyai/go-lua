package regression

import "testing"

func TestAnnotatedArrayCaptureRetainsElementDomain(t *testing.T) {
	checkBothModes(t, `
type Item = {[string]: any}
type Captures = {raised: {Item}, cleared: {Item}}
local function run(fn: (Captures) -> ())
 local raised: {Item} = {}
 local cleared: {Item} = {}
 pcall(function() fn({raised = raised, cleared = cleared}) end)
end
return run`, "")
}

func TestAnnotatedArrayCaptureRejectsWrongElementDomain(t *testing.T) {
	checkBothModes(t, `
local function run(fn: ({values: {string}}) -> ())
 local values: {number} = {}
 pcall(function() fn({values = values}) end)
end
return run`, "expected")
}

func TestAnnotatedMapCaptureRetainsKnownWrites(t *testing.T) {
	checkBothModes(t, `
local function consume(value: string) return value end
local values: {[string]: {[string]: unknown}} = {}
local names = {KEY = "key"}
values[tostring(names.KEY)] = {name = "ready"}
local key: string = "key"
local found = values[key]
if found ~= nil then return consume(found.name) end
return nil`, "")
}
