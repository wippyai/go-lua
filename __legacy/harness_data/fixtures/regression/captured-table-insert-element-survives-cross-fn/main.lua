type Suite = {name: string, count: number}

local suites = {}

local function register(name: string)
    table.insert(suites, {name = name, count = 0})
end

local function run()
    for _, s in ipairs(suites) do
        local n: string = s.name
        local c: number = s.count
    end
end

register("a")
run()
