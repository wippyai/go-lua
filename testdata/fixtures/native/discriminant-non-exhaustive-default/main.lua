-- One arm of the four-variant union is unhandled: the dispatch is not exhaustive
-- and needs a default lane, so the exhaustive grant is withheld.

type Push = { kind: "push", value: number }
type Pop = { kind: "pop" }
type Peek = { kind: "peek", depth: number }
type Clear = { kind: "clear" }
type Op = Push | Pop | Peek | Clear

local function label(op: Op): string
    local out: string = ""
    if op.kind == "push" then
        out = "push"
    elseif op.kind == "pop" then
        out = "pop"
    elseif op.kind == "peek" then
        out = "peek"
    end
    return out
end

return label
