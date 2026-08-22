-- Abstention is the whole contract. The payload arrives as an opaque
-- parameter, so nothing proves where it lives and nothing refutes it either:
-- no placement is derived, no alias is observed, no escape is proved.
--
-- With send-safety enabled the family must publish neither a refutation nor an
-- admission proof for this send. An unknown placement is the absence of an
-- answer, not a verdict, and the runtime copy fallback needs no diagnostic to
-- take effect.
local pid: string = "worker"

local function forward(payload: any)
    process.send(pid, "forward", payload)
end

local function forward_field(envelope: {body: any})
    process.send(pid, "forward-field", envelope.body)
end

return {forward = forward, forward_field = forward_field}
