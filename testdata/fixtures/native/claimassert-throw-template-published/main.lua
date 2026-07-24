-- A non-nil assertion on an optional parameter publishes exactly one throw
-- template: only nil fails and the operation allocates nothing.

local function require_id(id: string?): string
    return id!
end

return require_id("job-1")
