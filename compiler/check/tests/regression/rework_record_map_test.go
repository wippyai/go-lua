package regression

import "testing"

func TestRecordMapRequiresCompleteKeyEvidence(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[string]: number}) end
local function run(values: {known: number})
 consume(values)
end
run({known = 1, unseen = "s"})`, "expected {[string]: number}")
}

func TestCompleteLiteralAndMapComponentSupplyKeyEvidence(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[string]: number}) end
local function run(values: {[string]: number})
 consume(values)
end
consume({known = 1})
run({known = 1, extra = 2})`, "")
}

func TestCompleteRecordMapRejectsWrongValue(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[string]: number}) end
consume({known = 1, unseen = "s"})`, "expected {[string]: number}")
}
