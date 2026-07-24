-- EFFECT ROW: channel.select suspends the actor. The suspension lane must be
-- published explicitly at the site; it may never be inferred from the fact that
-- the callee happens to be a host-boundary entry.
type Tick = {
    seq: number,
}

local primary = nil :: Channel<Tick>
local retry = nil :: Channel<Tick>

local selected = channel.select {
    primary:case_receive(),
    retry:case_receive(),
}

return selected.value.seq
