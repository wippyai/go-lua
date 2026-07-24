-- A record field is one slot: a field initialized from an integer multiplication
-- that may overflow-promote carries the numeric union, never an arm narrowed from
-- a small observed input.

local function step(i: integer)
    return { doubled = i * 2 }
end

local small = step(3)
return small.doubled
