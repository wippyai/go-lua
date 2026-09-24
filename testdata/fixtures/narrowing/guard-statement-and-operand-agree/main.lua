type Shape = { kind: "a", v: string } | { kind: "b", v: number }
type Box = { inner: { v: string | number }? }

local function take_string(s: string): string
    return s
end

local function take_optional(s: string?): string?
    return s
end

local function statement(t: Shape): string
    local s = "x"
    if t.kind == "a" then
        s = t.v
    end
    local w: boolean = s -- expect-error: cannot assign string to boolean
    return take_string(s)
end

local function operand(t: Shape): string
    local s = t.kind == "a" and t.v or "x"
    local w: boolean = s -- expect-error: cannot assign string to boolean
    take_string(t.kind == "a" and t.v or "x")
    return take_string(s)
end

local function nested_statement(b: Box): string
    local s = "x"
    if b.inner then
        if type(b.inner.v) == "string" then
            s = b.inner.v
        end
    end
    local w: boolean = s -- expect-error: cannot assign string to boolean
    return take_string(s)
end

local function nested_operand(b: Box): string
    local s = b.inner and type(b.inner.v) == "string" and b.inner.v or "x"
    local w: boolean = s -- expect-error: cannot assign string to boolean
    take_string(b.inner and type(b.inner.v) == "string" and b.inner.v or "x")
    return take_string(s)
end

local function operand_in_statement(b: Box): string
    if b.inner then
        local s = type(b.inner.v) == "string" and b.inner.v or "x"
        local w: boolean = s -- expect-error: cannot assign string to boolean
        return take_string(s)
    end
    return "x"
end

local function dynamic_statement(row: any): string?
    if type(row) == "table" then
        if type(row.meta) == "table" then
            return take_optional(row.meta.id)
        end
    end
    return nil
end

local function dynamic_operand(row: any): string?
    local id = type(row) == "table" and type(row.meta) == "table" and row.meta.id or nil
    take_optional(type(row) == "table" and type(row.meta) == "table" and row.meta.id or nil)
    return take_optional(id)
end

return statement, operand, nested_statement, nested_operand, operand_in_statement, dynamic_statement, dynamic_operand
