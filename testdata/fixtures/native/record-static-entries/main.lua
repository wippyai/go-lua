-- Every entry of a static record literal binds to its solved source occurrence
-- and a storage class; the boolean entry uses the canonical tagged storage class.

local function echo(input: string?): { echo: string, ok: boolean }
    return { echo = input or "no input", ok = true }
end

return echo
