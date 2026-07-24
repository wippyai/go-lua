-- A loop that cannot iterate still needs a representable bound: zero, never a
-- defaulted one.
type Header = { id: number }

local headers: {Header} = {}
for i = 1, 0 do
    headers[i] = { id = i }
end

return headers
