local test = {}

local function format_value(val: any): string
    if type(val) == "string" then
        return string.format("%q", val)
    elseif type(val) == "table" then
        if val._tostring then
            return tostring(val:_tostring())
        else
            local str = "{"
            for k, v in pairs(val) do
                str = str .. (type(k) == "number" and "" or k .. "=") .. format_value(v) .. ","
            end
            return str .. "}"
        end
    else
        return tostring(val)
    end
end

-- Assertion functions

function test.eq(actual: any, expected: any, msg: string?)
    if actual ~= expected then
        error((msg or "assertion failed") .. ": expected " .. format_value(expected) .. ", got " .. format_value(actual), 2)
    end
end

function test.neq(actual: any, expected: any, msg: string?)
    if actual == expected then
        error((msg or "assertion failed") .. ": expected not " .. format_value(expected), 2)
    end
end

function test.ok(val: any, msg: string?): any
    if not val then
        error((msg or "assertion failed") .. ": expected truthy value, got " .. format_value(val), 2)
    end
    return val
end

function test.fail(msg: string?)
    error(msg or "assertion failed", 2)
end

function test.is_nil(val: any, msg: string?)
    if val ~= nil then
        error((msg or "assertion failed") .. ": expected nil, got " .. format_value(val), 2)
    end
end

function test.not_nil(val: any, msg: string?): any
    if val == nil then
        error((msg or "assertion failed") .. ": expected non-nil value", 2)
    end
    return val
end

return test
