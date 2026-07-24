local config = { retries = 3, name = "audit" }

local function scale(factor: integer): integer
	return factor * 2
end

local budget = scale(config.retries)
return budget
