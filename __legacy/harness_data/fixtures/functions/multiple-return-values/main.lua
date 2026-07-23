local function divmod(a: number, b: number): (number, number)
    return a // b, a % b
end
local q: number, r: number = divmod(17, 5)
return q + r
