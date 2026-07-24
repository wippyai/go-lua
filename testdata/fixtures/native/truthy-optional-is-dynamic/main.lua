-- An optional value's truthiness is a class, not a boolean: it is dynamic
-- nil-or-false, which is what tells the JIT to keep the test.
local function label(x: string?): string
    if x then
        return x
    else
        return "none"
    end
end

return label
