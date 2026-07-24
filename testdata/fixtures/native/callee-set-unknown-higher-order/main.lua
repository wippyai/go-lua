-- Contract: a function value passed as a call argument and invoked through the
-- parameter has no statically provable callee; the callee set at the indirect
-- site is Unknown, and the escape of the argument is what forbids Complete.

local function apply(fn: (number) -> number, v: number): number
    return fn(v)
end

local function double(x: number): number
    return x * 2
end

return apply(double, 21)
