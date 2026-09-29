local M = {}

M.USER_AGENT_PREFIX = "user_agent:"
M.USER_AGENT_NAMESPACE = "user_agent"

local function trim(value: any): string
    if type(value) ~= "string" then return "" end
    return (value:gsub("^%s+", ""):gsub("%s+$", ""))
end

function M.trim(value: any): string
    return trim(value)
end

function M.is_user(ref: any): boolean
    local raw = trim(ref)
    return raw:sub(1, #M.USER_AGENT_PREFIX) == M.USER_AGENT_PREFIX
end

function M.user(component_id: string): string
    return M.USER_AGENT_PREFIX .. trim(component_id)
end

-- Accept the canonical user-agent ref and the older bare component id. Other
-- namespaced refs are registry/system agents and intentionally return nil.
function M.user_component_id(ref: any): string?
    local raw = trim(ref)
    if raw == "" then return nil end
    if raw:sub(1, #M.USER_AGENT_PREFIX) == M.USER_AGENT_PREFIX then
        local id = raw:sub(#M.USER_AGENT_PREFIX + 1)
        if id == "" then return nil end
        return id
    end
    if raw:find(":", 1, true) then return nil end
    return raw
end

-- A bare component id is shorthand for a user agent; already-namespaced refs
-- pass through unchanged.
function M.normalize(ref: any): string
    local raw = trim(ref)
    if raw == "" then return "" end
    if raw:find(":", 1, true) then return raw end
    return M.user(raw)
end

return M

