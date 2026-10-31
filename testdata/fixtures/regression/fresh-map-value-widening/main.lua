-- From wippy.agent.compiler:compiler in kickside/platform/inbox/test.
local function set_from_list(values: {string}): {[string]: boolean}
    local out = {}
    for _, value in ipairs(values) do
        out[value] = true
    end
    return out
end

local narrow: {[string]: true} = { a = true }
local broad: {[string]: boolean} = narrow -- expect-error: cannot assign

return set_from_list({"one", "two"})
