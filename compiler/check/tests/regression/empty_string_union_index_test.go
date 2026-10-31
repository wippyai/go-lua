package regression

import "testing"

func TestValueSourceEmptyAliasedMapRead(t *testing.T) {
	checkBothModes(t, `
type Map = {[string]: any}
local function f(inputs: any): any
    local rows = type(inputs) == "table" and (inputs :: Map) or {}
    return rows.default or rows[""]
end
return f`, "")
}

func TestEmptyStringUnionIndexControls(t *testing.T) {
	checkBothModes(t, `
type Map = {[string]: number}
local function f(rows: Map | number)
 return rows[""]
end
return f`, "cannot index type")
	checkBothModes(t, `
type Map = {[string]: number}
local function f(flag: boolean): number
 local rows = flag and ({x = 1} :: Map) or {}
 return rows[""]
end
return f`, "cannot return")
}
