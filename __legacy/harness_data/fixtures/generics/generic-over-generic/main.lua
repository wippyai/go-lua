type Box<T> = { value: T }
type Pair<T> = { left: Box<T>, right: Box<T> }
local function both<T>(p: Pair<T>): T
    return p.left.value
end
local p: Pair<number> = { left = { value = 1 }, right = { value = 2 } }
return both(p)
