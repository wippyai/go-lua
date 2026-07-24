-- The parent mutates the table after the closure is constructed: the
-- transported element judgment is revoked at the write and withheld after it.

local function reader(): () -> number
    local buf: {number} = {}
    buf[1] = 10
    buf[2] = 20
    local read = function(): number
        return buf[1] + buf[2]
    end
    buf[2] = 0
    return read
end

return reader()
