type Type<T> = {
    decode: (any) -> T,
}

local M = {}

function M.decode<T>(data: string, witness: Type<T>): T
    return witness.decode(data)
end

function M.decode_map<T, U>(data: string, witness: Type<T>, fn: (T) -> U): U
    return fn(witness.decode(data))
end

function M.decode_many_map<T, U>(data: string, witness: Type<{T}>, fn: (T) -> U): {U}
    local out: {U} = {}
    for _, item in ipairs(witness.decode(data)) do
        table.insert(out, fn(item))
    end
    return out
end

return M
