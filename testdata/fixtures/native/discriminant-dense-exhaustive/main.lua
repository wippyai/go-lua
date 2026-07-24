-- Four record variants discriminated by a literal kind field with every arm
-- handled: the sequential string compares collapse to one bounded-index jump.

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
    elseif op.kind == "clear" then
        out = "clear"
    end
    return out
end

return label
