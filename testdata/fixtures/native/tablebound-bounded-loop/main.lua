-- A constructor inside a statically bounded loop becomes repeatable only with
-- an exact maximum occurrence count tied to the lifecycle coordinate.
type Header = { id: number }

local headers: {Header} = {}
for i = 1, 8 do
    headers[i] = { id = i }
end

return headers
