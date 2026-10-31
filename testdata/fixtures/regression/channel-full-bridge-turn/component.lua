local test_state = require("test_state")

local M = {}

M.ACCESS = { READ = 1, WRITE = 2, DELETE = 4, ADMIN = 8 }

local function matches_meta(row: any, meta_filters: any): boolean
    if type(meta_filters) ~= "table" then return true end
    local meta = type(row.meta) == "table" and row.meta or {}
    for key, value in pairs(meta_filters) do
        if tostring(meta[key]) ~= tostring(value) then return false end
    end
    return true
end

local function in_list(value: any, list: any): boolean
    if type(list) ~= "table" then return false end
    for _, v in ipairs(list) do
        if v == value then return true end
    end
    return false
end

-- list_system filters the component model by component_ids / impl_ids / meta,
-- mirroring the indexed core read the channel uses for routing and guards.
function M.list_system(args: any): any
    args = type(args) == "table" and args or {}
    local out: { any } = {}
    for _, row in pairs(test_state.state.components or {}) do
        local keep = true
        if type(args.component_ids) == "table" and #args.component_ids > 0 then
            if not in_list(row.component_id, args.component_ids) then keep = false end
        end
        if keep and type(args.impl_ids) == "table" and #args.impl_ids > 0 then
            if not in_list(row.impl_id, args.impl_ids) then keep = false end
        end
        if keep and not matches_meta(row, args.meta) then keep = false end
        if keep then
            out[#out + 1] = { component_id = row.component_id, impl_id = row.impl_id, meta = row.meta }
        end
    end
    return out
end

function M.set_meta(component_id: string, fields: any): (boolean, any)
    test_state.state.set_meta_calls[#test_state.state.set_meta_calls + 1] = {
        component_id = component_id,
        fields = fields,
    }
    return true, nil
end

return M

