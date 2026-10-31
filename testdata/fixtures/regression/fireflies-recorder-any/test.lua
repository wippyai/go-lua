local test = {}

function test.describe(name: string, fn: () -> ())
    fn()
end

function test.it(name: string, fn: () -> ())
    fn()
end

function test.run_cases(define_cases_fn: () -> ()): any
    define_cases_fn()
    return { passed = true }
end

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

function test.is_true(val: any, msg: string?)
    if val ~= true then
        error((msg or "assertion failed") .. ": expected true, got " .. format_value(val), 2)
    end
end

function test.is_false(val: any, msg: string?)
    if val ~= false then
        error((msg or "assertion failed") .. ": expected false, got " .. format_value(val), 2)
    end
end

function test.is_string(val: any, msg: string?): string
    if type(val) ~= "string" then
        error((msg or "assertion failed") .. ": expected string, got " .. type(val), 2)
    end
    return val
end

function test.is_number(val: any, msg: string?): number
    if type(val) ~= "number" then
        error((msg or "assertion failed") .. ": expected number, got " .. type(val), 2)
    end
    return val
end

function test.is_table(val: any, msg: string?): any
    if type(val) ~= "table" then
        error((msg or "assertion failed") .. ": expected table, got " .. type(val), 2)
    end
    return val
end

function test.is_function(val: any, msg: string?)
    if type(val) ~= "function" then
        error((msg or "assertion failed") .. ": expected function, got " .. type(val), 2)
    end
    return val
end

function test.is_boolean(val: any, msg: string?): boolean
    if type(val) ~= "boolean" then
        error((msg or "assertion failed") .. ": expected boolean, got " .. type(val), 2)
    end
    return val
end

function test.contains(str: any, substr: string, msg: string?): string
    if type(str) ~= "string" or not string.find(str, substr, 1, true) then
        error((msg or "assertion failed") .. ": expected string to contain '" .. tostring(substr) .. "'", 2)
    end
    return str
end

function test.matches(str: any, pattern: string, msg: string?): string
    if type(str) ~= "string" or not string.match(str, pattern) then
        error((msg or "assertion failed") .. ": expected string to match pattern '" .. tostring(pattern) .. "'", 2)
    end
    return str
end

function test.has_key(tbl: any, key: any, msg: string?): any
    if type(tbl) ~= "table" then
        error((msg or "assertion failed") .. ": expected table, got " .. type(tbl), 2)
    end
    if tbl[key] == nil then
        error((msg or "assertion failed") .. ": expected table to have key '" .. tostring(key) .. "'", 2)
    end
    return tbl[key]
end

function test.len(val: any, expected: number, msg: string?)
    local actual = #val
    if actual ~= expected then
        error((msg or "assertion failed") .. ": expected length " .. tostring(expected) .. ", got " .. tostring(actual), 2)
    end
end

function test.gt(a: any, b: number, msg: string?)
    if not (a > b) then
        error((msg or "assertion failed") .. ": expected " .. format_value(a) .. " > " .. format_value(b), 2)
    end
end

function test.gte(a: any, b: number, msg: string?)
    if not (a >= b) then
        error((msg or "assertion failed") .. ": expected " .. format_value(a) .. " >= " .. format_value(b), 2)
    end
end

function test.lt(a: any, b: number, msg: string?)
    if not (a < b) then
        error((msg or "assertion failed") .. ": expected " .. format_value(a) .. " < " .. format_value(b), 2)
    end
end

function test.lte(a: any, b: number, msg: string?)
    if not (a <= b) then
        error((msg or "assertion failed") .. ": expected " .. format_value(a) .. " <= " .. format_value(b), 2)
    end
end

function test.throws(fn: () -> (), msg: string?): any
    local ok, err = pcall(fn)
    if ok then
        error((msg or "throws failed") .. ": expected function to throw", 2)
    end
    return err
end

function test.has_error(val: any, err: any, msg: string?)
    if val ~= nil then
        error((msg or "has_error failed") .. ": expected nil result, got " .. format_value(val), 2)
    end
    if err == nil then
        error((msg or "has_error failed") .. ": expected error, got nil", 2)
    end
end

function test.no_error(val: any, err: any, msg: string?)
    if err ~= nil then
        error((msg or "no_error failed") .. ": unexpected error: " .. tostring(err), 2)
    end
end


-- Format error with stack trace into a structured object

return test
