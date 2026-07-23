local function zip<A, B>(a: A, b: B): { first: A, second: B }
    return { first = a, second = b }
end
local z = zip("x", 5)
local s: string = z.first
local n: number = z.second
return s .. tostring(n)
