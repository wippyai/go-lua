package regression

import "testing"

func TestFreshTableMapDomainSurvivesInference(t *testing.T) {
	checkBothModes(t, `
local function copy(source: {[string]: boolean}): {[string]: boolean}
 local out = {}
 for key, value in pairs(source) do out[key] = value end
 return out
end
local function captures(callback: ({raised: {[string]: boolean}, cleared: {[string]: boolean}}) -> ())
 callback({raised = {}, cleared = {}})
end
local function consume(value: {[string]: boolean}) end
consume({created = true})
return copy, captures`, "")
}

func TestPartialRecordHasNoMapDomain(t *testing.T) {
	checkBothModes(t, `
local function consume(value: {[string]: boolean}) end
local function run(value: {created: boolean}) consume(value) end
return run`, "expected {[string]: boolean}")
}

func TestFreshTableMapDomainRejectsWrongValue(t *testing.T) {
	checkBothModes(t, `
local function consume(value: {[string]: boolean}) end
consume({created = "wrong"})`, "expected {[string]: boolean}")
}
