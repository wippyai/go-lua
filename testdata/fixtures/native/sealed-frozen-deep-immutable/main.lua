-- A closed literal passed to table.freeze is deeply immutable: the seal covers the
-- nested table value, not only the outer allocation.

local config = {
    host = "localhost",
    limits = { retries = 3, timeout = 30 },
}
table.freeze(config)

return config.limits.retries
