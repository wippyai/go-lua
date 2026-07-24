-- One interior operand is any: the aggregate concat judgment is withheld
-- rather than published over the provable operands only.

local function join(a: string, b: any, c: string, d: string): string
    return a .. b .. c .. d
end

return join("w", 2, "x", "!")
