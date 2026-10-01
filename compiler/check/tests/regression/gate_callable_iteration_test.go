package regression

import "testing"

func TestGateCallableIterationKeepsLiteralDispatchCases(t *testing.T) {
	checkBothModes(t, `
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
`, "")
}
