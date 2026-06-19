-- A foreign-value write through a dynamic key into a closed record weakens the
-- declared fields the key could match: `pairs(item)` ranges its key over every
-- field name, so `item[key] = tostring(value)` can store a string into `count`.
-- The field's domain becomes `number | string`, so the closed-field contract is
-- broken and the diagnostic is pinned at the write site where the dynamic key
-- can target either closed field.
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end

local count: number = item.count
return count
