-- CLEAN positive, distilled from connection_negotiator.lua:64.
-- A single derived scalar; no heap allocation escapes.
type Opts = { proto: string, timeout: number }

local function negotiate(a: Opts, b: Opts): string
    local proto: string = a.proto        -- no Heap root; borrowed scalar
    if a.timeout < b.timeout then
        proto = b.proto
    end
    return proto                         -- a scalar string leaves the frame
end

return negotiate
