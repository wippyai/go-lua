package regression

import "testing"

func TestValueSourceMutatedEntryFieldStaysPresent(t *testing.T) {
	checkBothModes(t, `
local function handler(rows: {any})
    local m = {}
    for _, r in ipairs(rows) do
        local k = r.db
        if type(k) ~= "string" then k = "UNKNOWN" end
        if not m[k] then m[k] = { id = k, list = {} } end
        table.insert(m[k].list, { n = 1 })
    end
    for _, d in pairs(m) do
        table.sort(d.list, function(a, b) return a.n < b.n end)
    end
end
return handler`, "")
}

func TestValueSourceEntryFieldRejectionControls(t *testing.T) {
	checkBothModes(t, `
local function handler(rows: {any})
    local m = {}
    for _, r in ipairs(rows) do
        local k = r.db
        if type(k) ~= "string" then k = "UNKNOWN" end
        if not m[k] then m[k] = { id = k, list = {} } end
        table.insert(m[k].list, { n = "bad" })
    end
    for _, d in pairs(m) do
        local n: number = d.list[1].n
    end
end
return handler`, "cannot assign")
}

func TestValueSourcePresentEntryMissingFieldStillRejected(t *testing.T) {
	checkBothModes(t, `
local function f(flag: boolean)
    local m = {}
    if flag then m.x = {list = {1}} else m.x = {id = 1} end
    for _, d in pairs(m) do
        table.sort(d.list)
    end
end
return f`, "argument 1")
}
