-- Contract: a self-recursive function forms one call SCC; the row closes the
-- self edge and carries the argument and result representation together with the
-- exceptional completion the cycle can produce.

local function fib(n: number): number
    if n < 2 then
        return n
    end
    return fib(n - 1) + fib(n - 2)
end

return fib(10)
