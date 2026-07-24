-- The else edge of a nil guard carries information of its own: the subject is nil there, not
-- merely "not proven non_nil".

local function fallback(x: string?): string
    if x ~= nil then
        return x
    end
    return tostring(x)
end

return fallback
