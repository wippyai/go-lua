-- A shape_identity fact established by a field read must name write.field in its own
-- revocation set, be revoked by the store, and be reestablished at the read after it.
type Box = { v: number, tag: string }

local function total(b: Box): number
    local first = b.v
    b.v = first + 1
    local second = b.v
    return first + second
end

return total
