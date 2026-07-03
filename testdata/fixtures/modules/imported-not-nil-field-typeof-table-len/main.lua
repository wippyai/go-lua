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
-- departments is initialized here as a concrete non-empty sequence, so index 1 is
-- proven present after the not_nil + table guard. Unknown-length sequence reads
-- remain covered by assignment judgment unit tests.
local first: string = response.result.data.departments[1]

return count, first
