-- A self-write through a dynamic key preserves the closed record's declared
-- fields: `item[key] = value` where `(key, value)` are the same `pairs(item)`
-- iteration pair stores `item[key]` back to `item[key]`, so each field keeps its
-- type. `item.count` stays `number` and `item.name` stays `string`. This is the
-- sound companion to the foreign-write weakening: it must not over-strict.
local item = {
	count = 1,
	name = "ready",
}

for key, value in pairs(item) do
	item[key] = value
end

local count: number = item.count
local name: string = item.name
return count, name
