-- One interior operand is any: the aggregate concat judgment is withheld
-- rather than published over the provable operands only. The concat result
-- inherits that operand's boundary, so it cannot satisfy the declared string
-- return either.

local function join(a: string, b: any, c: string, d: string): string
    return a .. b .. c .. d -- expect-error
end

return join("w", 2, "x", "!")
