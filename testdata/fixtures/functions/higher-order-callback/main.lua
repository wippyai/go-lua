local function apply_twice<T>(f: (T) -> T, x: T): T
    return f(f(x))
end
local inc = function(n: number): number return n + 1 end
return apply_twice(inc, 0)
