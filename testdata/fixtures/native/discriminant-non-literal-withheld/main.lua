-- The discriminant is compared against a runtime string rather than literals: the
-- case set is not closed, so no dense-dispatch fact is published.

type Push = { kind: "push", value: number }
type Pop = { kind: "pop" }
type Op = Push | Pop

local function matches(op: Op, wanted: string): boolean
    return op.kind == wanted
end

return matches
