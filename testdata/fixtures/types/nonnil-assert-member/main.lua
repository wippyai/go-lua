type Box = { v: number }
local function f(b: Box?): number
    return (b!).v
end
return f({ v = 7 })
