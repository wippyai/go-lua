local function accept(fn: (a: string) -> string): string
    return fn("x")
end

local function bad(n: number): number
    return n
end

return accept(bad)
