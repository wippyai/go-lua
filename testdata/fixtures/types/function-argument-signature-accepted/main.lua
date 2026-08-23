local function accept(fn: (a: string) -> string): string
    return fn("x")
end

local function good(s: string): string
    return s
end

return accept(good)
