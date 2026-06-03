-- A foreign-value write through a dynamic key into a closed record weakens the
-- declared fields the key could match: `pairs(item)` ranges its key over every
-- field name, so `item[key] = tostring(value)` can store a string into `count`.
-- The field's domain becomes `number | string`, so the closed-field contract is
-- broken and the write/read is rejected (one error). The checker flags the
-- write site, the normal flow the read site, so the curated truth is an error
-- COUNT (manifest check.errors=1) rather than a line-pinned inline annotation.
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = tostring(value)
end

local count: number = item.count
return count
