package regression

import "testing"

// Callback synthesis and body checking share captured values.
func TestCallbackCaptureReadsValueAtCreation(t *testing.T) {
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"guarded counter with two callbacks", `
type Selection = {revision: string, items: {string}}
local catalog = {}
function catalog.read(): Selection return {revision = "1", items = {}} end
function catalog.resolve_open(refresh: () -> Selection?): Selection? return refresh() end
local function run(): ()
	local visible = catalog.read()
	local stale: Selection = {revision = visible.revision, items = {}}
	local refreshes = 0
	catalog.resolve_open(function()
		refreshes = refreshes + 1
		return refreshes == 1 and stale or visible
	end)
	refreshes = 0
	catalog.resolve_open(function()
		refreshes = refreshes + 1
		return visible
	end)
end
return run
`, ""},
		{"captured local read before its call result is bound elsewhere", `
type Selection = {revision: string}
local function read(): Selection return {revision = "1"} end
local function open(refresh: () -> Selection?) end
local function run(): ()
	local visible = read()
	local stale: Selection = {revision = "x"}
	local n = 0
	open(function()
		return n == 1 and visible or stale
	end)
	n = 0
end
return run
`, ""},
		{"captured counter escaping as a return is rejected", `
type Selection = {revision: string}
local function read(): Selection return {revision = "1"} end
local function open(refresh: () -> Selection?) end
local function run(): ()
	local visible = read()
	local n = 0
	open(function()
		return n == 1 and n or visible
	end)
	n = 0
end
return run
`, "argument 1: expected fun() -> Selection?"},
		{"incompatible assignment before creation is rejected", `
local function open(cb: () -> number) end
local function run(): ()
    local v = 1
    v = "s"
    open(function() return v end)
end
return run
`, "argument 1: expected fun() -> number"},
	} {
		t.Run(tt.name, func(t *testing.T) { checkBothModes(t, tt.code, tt.want) })
	}
}

func TestBeeCallbackCapturedCounter(t *testing.T) {
	checkBothModes(t, `-- Bee runtime-pin checker regression: bee.apps:catalog_test:383
-- Expected vs actual: Both callbacks return Selection; actual first return becomes unresolved | Selection.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Selection = {revision: string, items: {string}}
local test = {}
function test.it(name: string, run: () -> ()) run() end
local catalog = {}
function catalog.read(): Selection return {revision = "1", items = {}} end
function catalog.resolve_open(refresh: () -> Selection?): Selection? return refresh() end
local function run(): ()
    test.it("refresh", function()
        local visible = catalog.read() :: Selection
        local stale: Selection = {revision = visible.revision, items = {}}
        local refreshes = 0
        catalog.resolve_open(function()
            refreshes = refreshes + 1
            return refreshes == 1 and stale or visible
        end)
        refreshes = 0
        catalog.resolve_open(function()
            refreshes = refreshes + 1
            return visible
        end)
    end)
end
return run
`, "")
}

func TestCallbackCaptureStorageReadNilObligation(t *testing.T) {
	checkBothModes(t, `
local function mem_storage()
    local files = {}
    local S = {}
    function S:open(path, mode)
        if mode == "w" then
            local buf = {}
            return {
                write = function(_, s) buf[#buf + 1] = s end,
                read = function() return nil end,
                close = function() files[path] = table.concat(buf) end,
            }
        end
        local data = files[path]
        if not data then return nil, "missing" end
        return { read = function() return data end }
    end
    return S
end
local storage = mem_storage()
local f, missing = storage:open("stage/nodes", "w")
local absent: nil = missing
f:write("nodes")
f:close()
local read = storage:open("stage/nodes", "r")
read:read()
`, "cannot call method on optional value without nil check")
}
