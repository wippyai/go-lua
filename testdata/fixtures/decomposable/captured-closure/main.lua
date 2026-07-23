local opts = { a = 1, b = 2 }
local function read(): number
	return opts.a
end
return read()
