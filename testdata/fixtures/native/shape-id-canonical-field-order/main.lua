-- Two structurally identical record types written with their fields in different
-- textual order intern to one shape identity: field order is canonical, not source.

type Span = { start: number, stop: number }
type Range = { stop: number, start: number }

local function as_span(r: Range): Span
    return { start = r.start, stop = r.stop }
end

local a: Span = { start = 0, stop = 4 }
local b: Range = { stop = 4, start = 0 }
return as_span(b).stop + a.stop
