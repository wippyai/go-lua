local function assert_not_nil(value: any)
    if value == nil then error("missing") end
end

local function matcher(value: any): any
    return { check = function() assert_not_nil(value) end }
end

matcher = function(value: any): any
    return { check = function() end }
end

local absent = nil
matcher(absent).check()
local first = absent.field

local function conditional(value: any, enabled: boolean): any
    return { check = function()
        if enabled then assert_not_nil(value) end
    end }
end

local absent_again = nil
conditional(absent_again, false).check()
local second = absent_again.field

return { first = first, second = second }
