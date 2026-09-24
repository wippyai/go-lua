-- A guard on t[k] with a variable key proves the entry present for reads of
-- t[k] with the same key while neither t nor k changes
-- (llm claude client: appending stream deltas to the block at an index).
type Block = { thinking: string }

local function append_thinking(blocks: { [integer]: Block }, index: integer, chunk: string)
    if blocks[index] then
        blocks[index].thinking = blocks[index].thinking .. chunk
    end
end

local function lookup(t: { [string]: { id: string } }, k: string): string
    if t[k] ~= nil then
        return t[k].id
    end
    return ""
end

local function lookup_or_default(t: { [string]: { id: string } }, k: string): string
    if t[k] == nil then
        return ""
    end
    return t[k].id
end

local function stale_after_write(t: { [string]: { id: string } }, k: string): string
    if t[k] then
        t[k] = nil
        return t[k].id -- expect-error
    end
    return ""
end

return { append_thinking = append_thinking, lookup = lookup, lookup_or_default = lookup_or_default, stale_after_write = stale_after_write }
