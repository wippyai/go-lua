-- A table_element class fact over a locally filled numeric table is revoked by a call
-- through a function-typed parameter: that callee set is not Complete, so the element
-- class must be withheld at every read after the call.

local function run(notify: ({number}) -> ()): number
    local xs: {number} = { 1, 2, 3 }
    local first = xs[1]
    notify(xs)
    local second = xs[2]
    return (first or 0) + (second or 0)
end

return run
