-- An optional that comes from a call result (string.match returns string?) must
-- narrow under a presence guard exactly like a declared optional: the concat
-- operand nil-check reads the flow-solved presence. The guarded operand carries
-- no warning; the unguarded one still does.

local function narrowed(s: string): string
    local trimmed = s:match("^%s*(.-)%s*$")
    if not trimmed or trimmed == "" then
        return ""
    end
    return "value:" .. trimmed
end

local function unguarded(s: string): string
    local raw = s:match("p")
    return "value:" .. raw -- expect-warning: may be nil
end

return { narrowed = narrowed, unguarded = unguarded }
