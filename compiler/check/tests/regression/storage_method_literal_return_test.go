package regression

import (
	"strings"
	"testing"

	"github.com/wippyai/go-lua/compiler/check/tests/testutil"
)

// Minimal copy of kickside/platform/transfer/src/bundle_test.lua:11-23,137-139,163,194.
// The write branch returns a handle unconditionally; the read branch can return nil.
func TestStorageMethodLiteralModeReturn(t *testing.T) {
	source := `
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
`
	result := testutil.Check(source, testutil.WithStdlib())
	var writeError, readError bool
	for _, d := range result.Errors {
		if !strings.Contains(d.Message, "cannot call") || !strings.Contains(d.Message, "optional value without nil check") {
			t.Errorf("unexpected error at %d: %s", d.Position.Line, d.Message)
		}
		switch d.Position.Line {
		case 23, 24:
			writeError = true
		case 26:
			readError = true
		default:
			t.Errorf("unexpected error at %d: %s", d.Position.Line, d.Message)
		}
	}
	if writeError {
		t.Error("write-mode handle is unconditionally non-nil")
	}
	if !readError {
		t.Error("read-mode handle must retain its nil obligation")
	}
}

func TestStorageMethodLiteralModeDirect(t *testing.T) {
	source := `
local S = {}
function S:open(mode)
    if mode == "w" then return { write = function() end } end
    return nil, "missing"
end
local f, err = S:open("w")
local absent: nil = err
f:write()
`
	result := testutil.Check(source, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("direct method errors: %v", testutil.ErrorMessages(result.Errors))
	}
}

// The method shape comes from kickside/platform/transfer/src/bundle_test.lua:11-23;
// write calls at :139 are safe, while the read calls at :163 and :194 are not.
func TestStorageModeCallShapes(t *testing.T) {
	source := `
local function factory()
    local S = {}
    function S:open(path, mode)
        if mode == "w" then return { write = function() end, read = function() end } end
        return nil, "missing"
    end
    return S
end
local storage = factory()
local mode = "w"
local a, ae = storage:open("p", mode)
local anil: nil = ae
a:write()
local b, be = storage.open(storage, "p", "w")
local bnil: nil = be
b:write()
local opener = storage.open
local c, ce = opener(storage, "p", "w")
local cnil: nil = ce
c:write()
local read = storage:open("p", "r")
read:read()
local unknown_mode: string = "w"
local unknown = storage:open("p", unknown_mode)
unknown:read()
`
	result := testutil.Check(source, testutil.WithStdlib())
	var readError, unknownError bool
	for _, d := range result.Errors {
		if !strings.Contains(d.Message, "optional value without nil check") {
			t.Errorf("unexpected error at %d: %s", d.Position.Line, d.Message)
			continue
		}
		switch d.Position.Line {
		case 23:
			readError = true
		case 26:
			unknownError = true
		default:
			t.Errorf("unexpected optional error at %d: %s", d.Position.Line, d.Message)
		}
	}
	if !readError || !unknownError {
		t.Errorf("read and unknown modes must remain optional, got %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestStorageFactoryExportKeepsReturnCases(t *testing.T) {
	module := testutil.CheckAndExport(`
local S = {}
function S:open(path, mode)
    if mode == "w" then return { write = function() end } end
    return nil, "missing"
end
return S
`, "storage", testutil.WithStdlib())
	if module.HasError() {
		t.Fatalf("storage module errors: %v", testutil.ErrorMessages(module.Errors))
	}
	result := testutil.Check(`
local storage = require("storage")
local handle, err = storage:open("p", "w")
local absent: nil = err
handle:write()
`, testutil.WithStdlib(), testutil.WithModule("storage", module))
	if result.HasError() {
		t.Fatalf("exported method proof lost: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestStorageMethodReplacementDoesNotKeepProof(t *testing.T) {
	result := testutil.Check(`
local S = {}
function S:open(mode)
    if mode == "w" then return { write = function() end } end
    return nil, "missing"
end
S.open = function(self, mode) return nil, "replaced" end
local handle = S:open("w")
handle:write()
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 9 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("replaced method must retain exactly the nil obligation at the call: %v", testutil.ErrorMessages(result.Errors))
	}
}

func TestAlternativeCalleeCannotBorrowWriteProof(t *testing.T) {
	result := testutil.Check(`
local function good(mode)
    if mode == "w" then return { write = function() end } end
    return nil, "missing"
end
local function bad(mode) return nil, "missing" end
local function use(flag: boolean)
    local opener = bad
    if flag then opener = good end
    local handle = opener("w")
    handle:write()
end
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 11 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("alternative callee must retain exactly the nil obligation at the call: %v", testutil.ErrorMessages(result.Errors))
	}
}

// Minimal callable reassignment based on kickside/platform/transfer/src/bundle_test.lua:11-23,137-139.
func TestAlternativeCalleeGoodFirstCannotBorrowWriteProof(t *testing.T) {
	result := testutil.Check(`
local function good(mode)
    if mode == "w" then return { write = function() end } end
    return nil, "missing"
end
local function bad(mode) return nil, "missing" end
local function use(flag: boolean)
    local opener = good
    if flag then opener = bad end
    local handle = opener("w")
    handle:write()
end
`, testutil.WithStdlib())
	if len(result.Errors) != 1 || result.Errors[0].Position.Line != 11 || result.Errors[0].Message != "cannot call method on optional value without nil check" {
		t.Fatalf("reassigned callee must retain exactly the nil obligation at the call: %v", testutil.ErrorMessages(result.Errors))
	}
}

// Minimal copy of kickside/platform/transfer/src/bundle_test.lua:11-43,128-140.
func TestStorageWriteModeInsideCallbackLoop(t *testing.T) {
	result := testutil.Check(`
local test = require("test")
local function mem_storage()
    local files = {}
    local S = {}
    function S:open(path, mode)
        if mode == "w" then
            local buf = {}
            return {
                write = function(_, s) buf[#buf + 1] = s end,
                read = function() return nil end,
                seek = function() end,
                close = function() files[path] = table.concat(buf) end,
            }
        end
        local data = files[path]
        if not data then return nil, "not found: " .. tostring(path) end
        local pos = 1
        return {
            read = function(_, n: integer)
                if pos > #data then return "" end
                local chunk = string.sub(data, pos, pos + n - 1)
                pos = pos + #chunk
                return chunk
            end,
            write = function() end,
            seek = function(_, whence, off) if whence == "set" then pos = (off or 0) + 1 end end,
            close = function() end,
        }
    end
    function S:stat(path)
        local d = files[path]
        if not d then return nil end
        return { size = #d }
    end
    function S:remove(path) files[path] = nil end
    return S, files
end
local function define_tests()
    test.describe("bundle", function()
        test.it("stream", function()
            local storage = mem_storage()
            local nodes = "nodes"
            local big = string.rep("x", 600000)
            for _, p in ipairs({
                {path = "stage/nodes", bytes = nodes},
                {path = "stage/big", bytes = big},
            }) do
                local f = storage:open(p.path, "w")
                f:write(p.bytes)
                f:close()
            end
        end)
    end)
end
local run_cases = test.run_cases(define_tests)
return {run = function(options) return run_cases(options) end}
`, testutil.WithStdlib())
	if result.HasError() {
		t.Fatalf("write-mode loop errors: %v", testutil.ErrorMessages(result.Errors))
	}
}
