local function need_string(s: string): string
    return s
end

local function lo(): number
    return 1
end

local function hi(): number
    return 10
end

for i = lo(), hi() do
    need_string(42)
end
