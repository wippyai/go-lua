-- An ordered comparison that evaluates proves its operand is a number on
-- both outcomes, so the path where the comparison is false keeps the number
-- (simplefin pull_core: a positive-integer guard on tonumber's result).
local function days_ago(x: any, now: number): number?
    local days = tonumber(x)
    if days == nil then
        return nil
    end
    if days <= 0 then
        return nil
    end
    return now - days * 86400
end

local function guarded(x: any, now: number): number?
    local days = tonumber(x)
    if days == nil or days <= 0 or days ~= math.floor(days) then
        return nil
    end
    local as_text: string = days -- expect-error: cannot assign number to string
    return now - days
end

local function in_range(v: number?): boolean
    if v ~= nil and v > 10 then
        return true
    end
    return v == nil or v >= 0
end

return { days_ago = days_ago, guarded = guarded, in_range = in_range }
