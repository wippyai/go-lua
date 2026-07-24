-- The claim assert is never erased by optimism: on an operand the checker has
-- already proven non-nil the throw template is still published.

local function tag(id: string?): string
    if id == nil then
        return "anonymous"
    end
    return id!
end

return tag("job-1")
