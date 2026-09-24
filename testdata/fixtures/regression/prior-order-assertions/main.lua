-- From kickside.core.threads.persist:writer_test, cascade order assertions.
local test = {
    is_true = function(_actual: boolean) end,
    eq = function(_actual: any, _expected: any) end,
}

local steps: { {table: string} } = {
    {table = "kickside_projection_cursor"},
    {table = "kickside_effect_attempt"},
}
local at: { [string]: integer } = {}
for i, step in ipairs(steps) do at[step.table] = i end
test.is_true(at.kickside_effect_attempt > at.kickside_projection_cursor)
test.eq(at.kickside_effect_attempt, at.kickside_projection_cursor + 1)
