package regression

import "testing"

// design-record-to-map.md freezes this v1.6.2 false negative until v1.7.
func TestRecordMapInterimInexactValueHole(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[string]: number}) end
local function run(values: {known: number})
 consume(values)
end
run({known = 1, unseen = "s"})`, "")
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

// design-record-to-map.md: documented key-domain false negative, deferred to v1.7.
func TestRecordMapInterimHiddenNumericKeyHole(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[string]: unknown})
 for key in pairs(values) do local text: string = key end
end
local function run(record: {a: number}) consume(record) end
run({a = 1, [1] = "x"})`, "")
}

// design-record-to-map.md: documented mutable-view false negative, deferred to v1.7.
func TestRecordMapInterimWriteThroughViewHole(t *testing.T) {
	checkBothModes(t, `
local record: {a: number} = {a = 1}
local view: {[string]: unknown} = record
view.a = "str"
local value: number = record.a
return value`, "")
}

func TestRecordMapInterimKnownWrongKeyStillRejected(t *testing.T) {
	checkBothModes(t, `
local function consume(values: {[integer]: unknown}) end
local function run(record: {a: number}) consume(record) end
return run`, "expected {[integer]: unknown}")
}
