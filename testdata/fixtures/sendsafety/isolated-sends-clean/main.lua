-- The negative column of the send-safety family: the closest correct sends,
-- the ones a refutation must never reach.
--
-- owned_unaliased builds the payload, holds the only reference to it, and hands
-- that reference to the send: the value is fully owned and admissible to the
-- shared heap with no copy.
--
-- alias_dead_before_send does alias the payload, but every alias is local to
-- the sending fiber and dead at the send, so nothing observable survives the
-- transfer either.
local pid: string = "worker"

local function owned_unaliased(id: string)
    local payload = {id = id, attempts = 0}
    process.send(pid, "owned", payload)
end

local function alias_dead_before_send(id: string)
    local payload = {id = id, attempts = 0}
    local staging = payload
    staging.attempts = 1
    staging = {id = "scratch", attempts = 0}
    local _ = staging.id
    process.send(pid, "alias-dead", payload)
end

local function frozen_shared(id: string)
    local payload = table.freeze({id = id, attempts = 0})
    process.send(pid, "frozen", payload)
end

return {
    owned_unaliased = owned_unaliased,
    alias_dead_before_send = alias_dead_before_send,
    frozen_shared = frozen_shared,
}
