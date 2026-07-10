local function need_string(s: string): string
    return s
end

local function make(): { [string]: number }
    return {}
end

make()["field"] = 1
need_string(42)
