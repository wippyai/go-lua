-- From keeper.components.build:scanner in chestor/journal/local.
local function count_dir_bytes(vol: any, path: string, skip_dirs: unknown)
    if not vol:exists(path) then return 0, nil end
    if not vol:isdir(path) then
        local info = vol:stat(path)
        return (info and info.size) or 0, nil
    end
    local skips: {[string]: boolean} = {}
    local total = 0
    for entry in vol:readdir(path) do
        local child = path .. "/" .. entry.name
        if entry.type == "file" then
            local info = vol:stat(child)
            if info and info.size then total = total + info.size end
        elseif entry.type == "directory" and not skips[entry.name] then
            local sub = count_dir_bytes(vol, child, skips)
            total = total + sub
        end
    end
    return total
end

local function string_base(n: number)
    if n <= 0 then return "not a number" end
    local sub = string_base(n - 1)
    return sub + 1 -- expect-error: cannot perform arithmetic
end

return count_dir_bytes
