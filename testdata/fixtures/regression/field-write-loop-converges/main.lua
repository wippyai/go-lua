-- A loop writes a field back through a guard helper. The write gives the
-- record a new version whose written field comes from the write alone, so the
-- flow solve reaches a fixpoint.
local function ident(v: unknown): string
    if type(v) ~= "string" then
        error("invalid")
    end
    return v
end

local function run(rows: { { [string]: unknown } }): { { [string]: unknown } }
    for _, row in ipairs(rows) do
        row.a = ident(row.a)
    end
    return rows
end

return run
