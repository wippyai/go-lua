type Box<T> = { value: T, next: Box<T>? }

local function chain_len<T>(b: Box<T>?): number
    if b == nil then return 0 end
    return 1 + chain_len(b.next)
end

local b: Box<number> = { value = 1, next = { value = 2, next = nil } }
return chain_len(b)
