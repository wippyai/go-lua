type Rec = { name: string, size: integer }

local function early_return(c: string | Rec): string
	if type(c) == "string" then
		return c
	end
	local nm: string = c.name
	return nm
end

local function else_branch(c: string | Rec): string
	if type(c) == "string" then
		return c
	else
		local nm: string = c.name
		return nm
	end
end

type Element = { name: string, text: string?, children: {string | Element}? }

local function collect(el: Element, out: {string})
	table.insert(out, el.name)
	for _, c in ipairs(el.children or {}) do
		if type(c) == "string" then
			table.insert(out, c)
		else
			local tag: string = c.name
			table.insert(out, tag)
			local text: string = c.text or ""
			table.insert(out, text)
			collect(c, out)
		end
	end
end

local function guarded_variable(c: string | Rec): string
	if type(c) ~= "string" then
		local s: string = c -- expect-error: excluded by guard
		return s
	end
	return c
end

return {
	early_return = early_return,
	else_branch = else_branch,
	collect = collect,
	guarded_variable = guarded_variable,
}
