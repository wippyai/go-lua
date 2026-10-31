local M = {}
local function run()
    return M._registry.find({}) -- expect-error: cannot index type nil
end

run()
M._registry = { find = function() return {} end }
M.run = run
return M
