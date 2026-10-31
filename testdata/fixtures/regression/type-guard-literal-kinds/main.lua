-- type() of a literal-typed value reports the literal's base kind, so a
-- type() guard keeps literal values on the matching branch instead of
-- narrowing them away.
local function page_limit(p: { limit: any? }): integer
    local limit = 50
    if p.limit ~= nil then
        return 0
    end
    if type(limit) ~= "number" then
        return 0
    end
    local as_text: string = limit -- expect-error: cannot assign 50 to string
    return limit
end

local function default_label(): string
    local label = "none"
    if type(label) == "string" then
        local as_number: number = label -- expect-error: cannot assign "none" to number
        return label
    end
    return "unreachable"
end

local function flag(): boolean
    local enabled = true
    if type(enabled) ~= "boolean" then
        return false
    end
    local as_text: string = enabled -- expect-error: cannot assign true to string
    return enabled
end

return { page_limit = page_limit, default_label = default_label, flag = flag }
