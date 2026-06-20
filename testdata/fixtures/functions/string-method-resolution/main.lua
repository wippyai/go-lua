-- Method form resolves to the same precise type as the global form.
local function upper_method(): string
    return ("hello"):upper()
end
local function upper_global(): string
    return string.upper("hello")
end

-- len returns an integer, usable as a number.
local function length(s: string): number
    return s:len()
end

-- sub and rep return string; chaining composes string methods.
local function transform(s: string): string
    return s:sub(2, 4):rep(2):lower()
end

-- A string-compatible alias resolves string methods identically.
type Text = string
local function shout(t: Text): string
    return t:upper()
end

-- The return type is precise, not any: a string result is not a number.
local function precise(s: string): number
    return s:upper() -- expect-error
end

return upper_method, upper_global, length, transform, shout, precise
