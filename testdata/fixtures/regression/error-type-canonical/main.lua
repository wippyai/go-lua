-- `error` and `Error` name the one runtime error type: the value errors.new,
-- errors.wrap and host modules return, with kind, message, retryable,
-- details and stack methods.
local function fail(): (string?, error?)
    return nil, errors.new({ message = "bad", kind = errors.INVALID })
end

local function describe_error(e: Error): string
    local kind: string = e:kind()
    local retryable: boolean? = e:retryable()
    local details: { [string]: any }? = e:details()
    local stack: string = e:stack()
    return kind .. ":" .. e:message() .. tostring(retryable) .. tostring(details) .. stack
end

local function wrap(e: error): error
    return errors.wrap(e, "while failing")
end

-- Errors convert to strings through tostring and concatenation.
local function render(e: error): string
    return "failed: " .. e .. " / " .. tostring(e)
end

local value, err = fail()
if err then
    local text: string = describe_error(wrap(err)) .. render(err)
    print(text, errors.is(err, errors.INVALID))
else
    local s: string? = value
    print(s)
end

-- pcall reports whatever value was raised.
local ok, raised = pcall(function() error(errors.new("boom")) end)
if not ok then
    print(tostring(raised))
end

-- A plain string is not an error value: it has no error methods.
local function string_error(): (string?, error?)
    return nil, "plain message" -- expect-error: expected Error?
end

local bad_arg = errors.new(42) -- expect-error: argument 1
local no_message = errors.new({ kind = errors.INVALID }) -- expect-error: argument 1

return { fail = fail, string_error = string_error, bad_arg = bad_arg, no_message = no_message }
