-- The loop bound is a runtime value, so no maximum occurrence count exists and
-- the constructor bound fails closed instead of guessing a maximum.
type Header = { id: number }

local function build(n: number): {Header}
    local headers: {Header} = {}
    for i = 1, n do
        headers[i] = { id = i }
    end
    return headers
end

return build
