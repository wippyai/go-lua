local function need_number(n: number): number
    return n
end

-- Singleton-typed local on the right side of a type() comparison.
local kind: "number" = "number"
local v: number | string = 5
if type(v) == kind then
    need_number(v)
end

-- Literal-typed field of a structure on the right side.
type Spec = { tag: "number", value: number | string }
local function check(s: Spec): number
    if type(s.value) == s.tag then
        return need_number(s.value)
    end
    return 0
end

check({ tag = "number", value = 7 })
