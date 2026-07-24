-- tostring at this site is the canonical global binding: the builtin row joins
-- to the call site, carries the integer input and states one string result.

local labels: {string} = {}
local i: integer = 1

while i <= 8 do
    labels[i] = tostring(i)
    i = i + 1
end

return labels
