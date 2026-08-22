-- The whole declared math surface, grouped by what each member answers. The
-- two subtype-sensitive members answer optionals: tointeger answers nothing for
-- a value with no integer representation, and type answers nothing for a value
-- that is not a number at all, so both nil arms are read before the value is
-- used.
local function rounding(x: number): number
    return math.abs(x) + math.ceil(x) + math.floor(x) + math.fmod(x, 2) +
        math.mod(x, 3) + math.pow(x, 2) + math.sqrt(math.abs(x)) + math.exp(x)
end

local function splitting(x: number): number
    local whole, fraction = math.modf(x)
    local mantissa, exponent = math.frexp(x)
    return whole + fraction + mantissa + exponent + math.ldexp(mantissa, 2)
end

local function trigonometry(x: number): number
    return math.sin(x) + math.cos(x) + math.tan(x) +
        math.asin(x) + math.acos(x) + math.atan(x) + math.atan2(x, 1) +
        math.sinh(x) + math.cosh(x) + math.tanh(x) +
        math.deg(x) + math.rad(x) + math.log(math.abs(x) + 1) + math.log10(math.abs(x) + 1)
end

local function extrema(x: number, y: number): number
    math.randomseed(x)
    return math.max(x, y) + math.min(x, y) + math.random(1, 10) + math.random()
end

local function subtype(value: any): string
    local kind = math.type(value)
    if kind == nil then
        return "not a number"
    end
    local exact = math.tointeger(value)
    if exact == nil then
        return kind .. ":inexact"
    end
    if math.ult(exact, math.maxinteger) then
        return kind .. ":below-max"
    end
    return kind .. ":" .. tostring(exact)
end

local function constants(): number
    return math.pi + math.huge + math.maxinteger + math.mininteger
end

return {
    rounding = rounding,
    splitting = splitting,
    trigonometry = trigonometry,
    extrema = extrema,
    subtype = subtype,
    constants = constants,
}
