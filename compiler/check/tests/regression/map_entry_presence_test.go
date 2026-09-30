package regression

import "testing"

func TestValueSourceGuardedMapEntryPresence(t *testing.T) {
	checkBothModes(t, `
local function f(key: string, delta: number)
    local acc: {[string]: {required: number, complete: number}} = {}
    if not acc[key] then acc[key] = {required = 0, complete = 0} end
    local a = acc[key]
    a.required = a.required + delta
    return a.required
end
return f`, "")
}

func TestMapEntryPresenceControls(t *testing.T) {
	checkBothModes(t, `
local function f(acc: {[string]: {required: number}}, key: string)
 local a = acc[key]
 return a.required + 1
end
return f`, "cannot perform arithmetic")
	checkBothModes(t, `
local function f(acc: {[string]: {required: number}}, key: string, flag: boolean)
 if flag and not acc[key] then acc[key] = {required = 0} end
 local a = acc[key]
 return a.required + 1
end
return f`, "cannot perform arithmetic")
	checkBothModes(t, `
local function f(acc: {[string]: {required: number}}, key: string)
 if not acc[key] then acc[key] = {required = 0} end
 acc[key] = nil
 local a = acc[key]
 return a.required + 1
end
return f`, "cannot perform arithmetic")
}
