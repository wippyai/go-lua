-- The frontend flattens a .. b .. c .. d into one concat site: all four
-- operands carry site-bound classes and the operand list stays ordered.

local function join(a: string, b: string, c: string, d: string): string
    return a .. b .. c .. d
end

return join("w", "-", "x", "!")
