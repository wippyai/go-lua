-- The whole declared utf8 surface. len and offset answer optionals, so the
-- malformed-input arm is ruled out before the position is used; codes is an
-- iterator over the string it was given, and charpattern is a declared literal.
local function measure(text: string): number
    local count, failure = utf8.len(text)
    if count == nil then
        if failure == nil then
            return -1
        end
        return -failure
    end
    return count
end

local function first_codepoint(text: string): number
    local start = utf8.offset(text, 1)
    if start == nil then
        return 0
    end
    return utf8.codepoint(text, start)
end

local function rebuild(text: string): string
    local out = ""
    for position, code in utf8.codes(text) do
        out = out .. tostring(position) .. utf8.char(code)
    end
    return out .. utf8.charpattern
end

return {measure = measure, first_codepoint = first_codepoint, rebuild = rebuild}
