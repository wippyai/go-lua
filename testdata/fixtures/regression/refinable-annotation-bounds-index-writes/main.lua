-- A local annotated with a refinable type such as {any} is refined by the
-- writes into it only within the annotation: an index write whose key is not
-- known to be an integer leaves it {any} instead of turning it into a map
-- (spiralscout.meetings.extract validate_impl dedup, kickside.llm_ocr
-- stage_implementation parallel_map).
local channel = require("channel")

local function dedup(facts: { any }): { any }
    local kept: { any } = {}
    for _, raw in ipairs(facts) do
        local duplicate_of: number? = nil
        for i, other in ipairs(kept) do
            if other == raw then
                duplicate_of = i
                break
            end
        end
        if duplicate_of == nil then
            kept[#kept + 1] = raw
        else
            kept[duplicate_of :: number] = raw
        end
    end
    return kept
end

-- Bounded-concurrency map: page transcription is network-bound (one vision call each),
-- so fan the pages across a few cooperative workers and overlap the calls. Results are
-- placed back by index, so page order is preserved regardless of completion order.
local function parallel_map(items: { any }, fn: any, concurrency: integer): { any }
    local n = #items
    local out: { any } = {}
    if n == 0 then return out end
    local workers = concurrency
    if workers > n then workers = n end
    if workers < 1 then workers = 1 end

    local work = channel.new(n)
    local results = channel.new(n)
    for i = 1, n do work:send({ index = i, value = items[i] }) end
    work:close()

    for _ = 1, workers do
        coroutine.spawn(function()
            while true do
                local job, ok = work:receive()
                if not ok then return end
                local call_ok, value = pcall(fn, job.value)
                results:send({ index = job.index, value = call_ok and value or nil })
            end
        end)
    end

    for _ = 1, n do
        local res = results:receive()
        out[res.index] = res.value
    end
    return out
end


return { dedup = dedup, parallel_map = parallel_map }
