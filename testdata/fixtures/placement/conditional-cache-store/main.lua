type Row = {
    id: string,
    meta: {
        source: string,
    },
}

local cache: {[string]: Row} = {}
process.send("worker-1", "cache.ready", cache)

local function build(id: string, remember: boolean): Row
    local row: Row = {
        id = id,
        meta = {
            source = "builder",
        },
    }
    if remember then
        cache[row.id] = row
    end
    return row
end

local row = build("x", os.clock() > 0)
print(row.id)
