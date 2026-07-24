-- An opaque callee reaches the element domain between the fill and the read,
-- so the element class is revoked at the call and not reestablished.
local function build(sink: (any) -> ()): number?
    local xs: {number} = {}
    for i = 1, 4 do
        xs[i] = i
    end
    sink(xs)
    return xs[2]
end

return build
