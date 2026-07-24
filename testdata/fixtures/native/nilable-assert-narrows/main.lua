-- The non-nil assertion operator and a call to assert both publish nilability non_nil at the
-- point after them; each is one nil check the JIT does not emit.
type Row = { id: string? }

local function ids(r: Row, x: string?): string
    local a = r.id!
    assert(x)
    local b = x
    return a .. b
end

return ids
