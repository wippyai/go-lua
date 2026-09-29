local component = require("component")
local M = {}

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s*(.-)%s*$", "%1"))
end

function M.require_access(crm_id: string, access: integer): (boolean?, string?)
    if trim(crm_id) == "" then return nil, "crm_id required" end
    local _, err = component.validate_access(crm_id, access)
    if err then return nil, "crm access denied: " .. tostring(err) end
    return true, nil
end

function M.append(_crm_id: string, _events: any): (any?, string?)
    return {}, nil
end

return M
