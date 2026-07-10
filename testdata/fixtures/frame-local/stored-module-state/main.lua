local module_state: { [string]: { value: integer } } = {}

local function store()
    local scratch = {
        value = 1,
    }
    module_state["scratch"] = scratch
end

store()
