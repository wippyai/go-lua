-- Lua's # returns a border, not a count. A non-negative integer n is a border of
-- t when (n == 0 or t[n] ~= nil) and t[n + 1] == nil, and # may return any border
-- the table has. The table below has t[1] and t[3] set and t[2] nil, so its borders
-- are 1 and 3: 0 is not one because t[1] is present, and 2 is not one because t[2]
-- is absent. A length guard therefore constrains only the border it selects, never
-- the slots underneath it. No metatable is installed anywhere here, so # is the raw
-- length and the border definition is the whole contract.

-- #t >= 3 selects the border 3. It says nothing about slot 2, which was never
-- written.
local function hole_below_border(): string
    local t: {string} = {}
    t[1] = "a"
    t[3] = "c"
    if #t >= 3 then
        local v: string = t[2] -- expect-error
        return v
    end
    return ""
end

-- The hole is what makes the read optional, not the guard: slot 3 holds the value
-- assigned to it whichever border # reports.
local function written_slot(): string
    local t: {string} = {}
    t[1] = "a"
    t[3] = "c"
    if #t >= 3 then
        local v: string = t[3]
        return v
    end
    return ""
end

-- The border itself is occupied by definition. Both borders of this table are 1
-- and 3, and t[1] and t[3] are strings, so t[#t] cannot be nil even though the
-- table is holey.
local function border_slot(): string
    local t: {string} = {}
    t[1] = "a"
    t[3] = "c"
    local v: string = t[#t]
    return v
end

-- An opaque array may be empty. Then 0 is its border, and t[0] is nil, so the
-- border read is optional without a floor.
local function unguarded_border(t: {string}): string
    local v: string = t[#t] -- expect-error
    return v
end

return hole_below_border, written_slot, border_slot, unguarded_border
