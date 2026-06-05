type Type<T> = {
    decode: (any) -> T,
}

local M = {}

function M.decode<T>(data: string, witness: Type<T>): T
    return witness.decode(data)
end

return M
