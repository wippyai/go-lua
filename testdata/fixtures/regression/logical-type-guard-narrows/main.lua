-- A type() guard on the left of `and` narrows the right operand, and a
-- negated guard on the left of `or` narrows it too (keeper changeset
-- edit_dispatch: state = type(c.state) == "string" and c.state or nil).
type Row = {id: string, state: string?}

local function to_row(c: {[string]: unknown}): Row
    local id = type(c.id) == "string" and c.id or ""
    return {
        id = id,
        state = type(c.state) == "string" and c.state or nil,
    }
end

local function label(v: unknown): string
    return type(v) ~= "string" and "?" or v
end

return { to_row = to_row, label = label }
