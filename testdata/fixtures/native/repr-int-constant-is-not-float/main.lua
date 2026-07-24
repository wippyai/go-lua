-- The literal `2` is an integer machine word even though the declared type
-- `number` also admits floats; the constant must not be read as a float.
local function grow(x: number): number
    local step: number = 2
    return x + step
end

return grow
