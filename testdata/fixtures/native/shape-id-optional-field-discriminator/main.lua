-- An optional field is a physical discriminator: until the presence bit or the two
-- distinct shapes are published explicitly, no single shape identity holds across
-- a path that carries the field and a path that does not.

type Entry = { id: string, tags: {string}? }

local function tagged(id: string): Entry
    return { id = id, tags = { "a" } }
end

local function bare(id: string): Entry
    return { id = id }
end

local function count(e: Entry): number
    local t = e.tags
    if t == nil then
        return 0
    end
    return #t
end

return count(tagged("x")) + count(bare("y"))
