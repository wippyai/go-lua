-- Every operand is a string, so the concat dispatches primitively and no
-- operand can enter __concat.

local function greet(name: string): string
    return "Hello, " .. name .. "!"
end

return greet("world")
