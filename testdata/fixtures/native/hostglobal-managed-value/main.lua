-- A host value carrying a table is managed: ownership, rooting and release
-- must be published. The immediate integer beside it does not license that.

local ticks: integer = 42
local handle = stream.open("input")
local id: string = handle.id

return id, ticks
