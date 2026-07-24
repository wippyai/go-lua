-- The operand class survives the logical selection: the selected value is a
-- string at the concat site, so all three operands stay site-bound strings.

local function greet(name: string?): string
    return "Hello, " .. (name or "Anonymous") .. "!"
end

return greet(nil)
