-- `//` on two integer operands yields an integer result carrier; the contrast
-- with `/` and `^` is what keeps the per-instruction result rule honest.
local function halves(a: integer, b: integer): integer
    local d = a // b
    return d
end

return halves
