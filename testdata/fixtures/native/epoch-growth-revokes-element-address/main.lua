-- An append that can reallocate backing storage revokes the element fact taken before it:
-- a raw element address does not survive growth, so the fact must be reestablished.

local function run(): number
    local xs: {number} = { 1, 2, 3 }
    local first = xs[1]
    xs[#xs + 1] = 4
    local second = xs[1]
    return (first or 0) + (second or 0)
end

return run
