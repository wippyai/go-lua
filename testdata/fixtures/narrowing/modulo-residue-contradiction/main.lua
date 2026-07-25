-- Two residues of the same modulus are exclusive: inside the x % 2 == 0 arm the
-- test x % 2 == 1 is never true, so its then arm is an instruction stream the
-- JIT never emits.
local function classify(x: integer): string
    if x % 2 == 0 then
        if x % 2 == 1 then
            return "impossible"
        end
        return "even"
    end
    return "odd"
end

return classify
