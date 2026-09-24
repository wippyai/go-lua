-- A closure sees a parameter typed from its call sites as the function body
-- does (wippy test runner: run_test's entry read inside a pcall callback).
local function call(name: string): any
    return name
end

local function run_test(entry)
    local direct = call(entry.id)
    local ok, result = pcall(function()
        return call(entry.id)
    end)
    return ok, result, direct
end

local function run_suite(tests: any[])
    for _, test in ipairs(tests) do
        run_test(test)
    end
end

return run_suite
