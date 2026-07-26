type Event = {kind: string, payload: string?}

-- A declared callable whose body returns an annotated local.
local function make(): fun(): Event
    return function()
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end
local declared_callable: {Event} = { (make())() }

-- A declared return, same body shape.
local function direct(): Event
    local e: Event = {kind = "metric", payload = nil}
    return e
end
local declared_return: {Event} = { direct() }

-- An inferred return: the annotated local's declaration is the contract that
-- leaves the body, not the literal the constructor happened to produce.
local function inferred()
    local e: Event = {kind = "metric", payload = nil}
    return e
end
local inferred_element: {Event} = { inferred() }
local inferred_value: Event = inferred()

-- The same body reached through an inferred callable return.
local function make_inferred()
    return function()
        local e: Event = {kind = "metric", payload = nil}
        return e
    end
end
local nested_element: {Event} = { (make_inferred())() }
local nested_value: Event = (make_inferred())()

-- A container declaration types every slot the body fills, so an element the
-- loop wrote at a key the analysis cannot resolve stays the declared element.
local function collect(source: {number}): {number}
    local out: {number} = {}
    for index, value in ipairs(source) do
        out[index] = value
    end
    return out
end
local collected: {number} = collect({1, 2, 3})

-- Fail-closed: nothing checked an unannotated local, so its literal shape is
-- what leaves the body.
local function unannotated()
    local e = {kind = "metric"}
    return e
end
local literal = unannotated()
local literal_kind: number = literal.kind -- expect-error: cannot assign literal.kind
-- The same literal read straight off the call result, with no cell of its own.
local inline_kind: number = (unannotated()).kind -- expect-error: cannot assign unannotated(...).kind

-- Fail-closed: a cast is a claim, not a checked declaration.
local function casted(raw: any)
    local e = raw :: Event
    return e
end
local laundered: Event = casted(nil) -- expect-error: is not proven

-- The literal that violates its annotation refutes at the annotation itself.
local function violating()
    local e: Event = {kind = 12, payload = nil} -- expect-error: cannot assign e.kind
    return e
end

return "ok"
