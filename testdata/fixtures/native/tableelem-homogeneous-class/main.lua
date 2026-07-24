-- Every writer into the element domain stores number and the writer set is
-- closed, so the element class holds over the whole dense prefix.
local xs: {number} = {}
for i = 1, 8 do
    xs[i] = i * 2
end

return xs
