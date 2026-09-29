-- math.max and math.min return one of their arguments, so integer arguments
-- give an integer (keeper flow repo: content:sub(math.max(1, pos - 60), ...)).
local function snippet(content: string, query: string): string
    local pos = content:lower():find(query:lower(), 1, true)
    local start_i = math.max(1, (pos or 1) - 60)
    local end_i = math.min(#content, (pos or 1) + #query + 120)
    return content:sub(start_i, end_i)
end

local function ratio(a: number, b: integer): number
    return math.max(a, b)
end

return { snippet = snippet, ratio = ratio }
