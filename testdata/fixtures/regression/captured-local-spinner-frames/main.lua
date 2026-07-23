-- Derived from wippy-golua-seam/tests/app/src/runner.lua.

local reset = "\027[0m"
local function cyan(s: string): string return "\027[36m" .. s .. reset
end

local spinner_frames = {"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

local function run_suite(tests: {any})
    for i, _entry in ipairs(tests) do
        local spinner = cyan(spinner_frames[((i - 1) % #spinner_frames) + 1])
        local painted: string = spinner
    end
end

return run_suite
