type Item = { label: string | number, size: integer }

local function field_guard(it: Item): string
	if type(it.label) ~= "string" then
		local s: string = it.label -- expect-error: excluded by guard
		return s
	end
	local size: integer = it.size
	return tostring(size)
end

return { field_guard = field_guard }
