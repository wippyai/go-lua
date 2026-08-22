-- Every declared assert2 classifier is called and its declared answer is read.
-- The two refuting members answer their subject refined, because ruling the
-- other case out is what the assertion did; each classifier answers the type
-- its name asserts, and contains answers the subject string it searched.
local assert2 = require("assert2")

local function classify(row: any): string
    local present = assert2.ok(row, "row is present")
    local subject = assert2.not_nil(present, "row is not nil")

    assert2.eq(1, 1, "one equals one")
    assert2.neq(1, 2, "one differs from two")
    assert2.is_nil(nil, "the absent value is nil")

    local text = assert2.is_string(subject, "row reads as a string")
    local count = assert2.is_number(subject, "row reads as a number")
    local rows = assert2.is_table(subject, "row reads as a table")
    local callback = assert2.is_function(subject, "row reads as a function")
    local flag = assert2.is_boolean(subject, "row reads as a boolean")
    local matched = assert2.contains(text, "id", "the text carries id")

    return matched .. "/" .. tostring(count) .. "/" .. type(rows) .. "/" .. type(callback) .. "/" .. tostring(flag)
end

return classify
