-- An operand whose declared type admits a table carrying __concat cannot be
-- proved to dispatch primitively.

type Tag = {label: string}

local mt = {
    __concat = function(a: any, b: any): string return "tag" end,
}

local function render(t: Tag): string
    setmetatable(t, mt)
    return "tag:" .. t
end

return render({label = "release"})
