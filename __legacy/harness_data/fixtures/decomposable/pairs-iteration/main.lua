local opts = { a = 1, b = 2 }
local total = 0
for _, value in pairs(opts) do
	total = total + value
end
return total
