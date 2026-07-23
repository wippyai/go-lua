local config = {
	retry_count = 3,
	timeout_ms = 2500,
	enabled = true,
}

local budget = config.retry_count * config.timeout_ms
if config.enabled then
	return budget
end
return 0
