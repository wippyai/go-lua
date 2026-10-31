local automation_types = require("types")

local M = {}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

local function decode_json_map(raw: any): automation_types.Map
    if type(raw) == "table" then return raw :: automation_types.Map end
    return {}
end

local function row_to_binding(row: any): automation_types.Map
    local r = type(row) == "table" and (row :: automation_types.Map) or {}
    return {
        binding_id = tostring(r.binding_id or ""),
        lowering_state = decode_json_map(r.lowering_state),
    }
end

local function get_binding_row(binding_id: string, rows: any): (automation_types.Map?, string?)
    local id = trim(binding_id)
    if id == "" then return nil, "binding_id is required" end
    if type(rows) ~= "table" or not (rows :: { any })[1] then return nil, nil end
    return row_to_binding((rows :: { any })[1]), nil
end

function M.get_binding(binding_id: string, rows: any): (automation_types.Map?, any)
    local row, err = get_binding_row(binding_id, rows)
    if err then return nil, err end
    if not row then return nil, "binding not found: " .. tostring(binding_id) end
    return row, nil
end

return M
