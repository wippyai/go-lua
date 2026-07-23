-- A mere truthy guard proves the field is present, not that it is a string, so
-- it stays gradual and is not assignable to a concrete scalar.
local function run(block: any)
    if block.text then
        local s: string = block.text -- expect-error
        return s
    end
    return nil
end

-- `type(x.f) == "table"` proves table-ness but nothing about element types, so the
-- field stays gradual and is not assignable to a concrete typed container.
local function rows(block: any)
    if type(block.items) == "table" then
        local labels: {string} = block.items -- expect-error
        return labels
    end
    return nil
end

local a = run({text = "hi"})
local b = rows({items = {}})
return a, b
