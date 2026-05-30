local test = require("test")

type Response = {
    result: {
        data: {
            departments: {string}?,
        },
    },
}

local response: Response = {
    result = {
        data = {
            departments = { "engineering", "product" },
        },
    },
}

test.not_nil(response.result.data.departments, "departments required")
test.eq(type(response.result.data.departments), "table", "departments should be a table")

local count: number = #response.result.data.departments
-- departments is the typed sequence {string}?: not_nil + #-read remove the outer
-- optionality of the field, but a typed sequence has unknown runtime length, so an
-- arbitrary index read departments[1] is string? and assigning it to a non-optional
-- string is soundly rejected.
local first: string = response.result.data.departments[1] -- expect-error: cannot assign string? to string

return count, first
