-- The right side of t[k] = t[k] + 1 reads t[k] before the write: a guard on
-- t[k] proves the entry present for that read (spiralscout.crm policy summary).
local function summary(outcomes: any): any
    local out = { applied = 0, skipped = 0, review_required = 0, duplicate = 0 }
    for _, outcome in ipairs(type(outcomes) == "table" and outcomes or {}) do
        local key = type(outcome) == "table" and tostring((outcome :: any).outcome or "") or ""
        if out[key] ~= nil then out[key] = out[key] + 1 end
    end
    return out
end

local function bump(counts: {[string]: integer}, key: string)
    if counts[key] then
        counts[key] = counts[key] + 1
    end
end

local function stale_after_write(counts: {[string]: integer}, key: string)
    if counts[key] ~= nil then
        counts[key] = nil
        counts[key] = counts[key] + 1 -- expect-error
    end
end

return { summary = summary, bump = bump, stale_after_write = stale_after_write }
