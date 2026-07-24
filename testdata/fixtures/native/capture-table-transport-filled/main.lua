-- The parent fills the table before constructing the closure: initialization,
-- element class and dense prefix are carried into the child by the capture.

local function reader(): () -> number
    local buf: {number} = {}
    buf[1] = 10
    buf[2] = 20
    buf[3] = 30
    return function(): number
        return buf[1] + buf[3]
    end
end

return reader()
