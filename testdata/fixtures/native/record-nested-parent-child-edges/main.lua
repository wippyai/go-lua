-- The nested table value of the outer record gets its own parent/child edge row;
-- the edge is closed by the construction, not inferred from lexical nesting.

local function multi_arg_process(a: string, b: string, c: string)
    return {
        args = { a = a, b = b, c = c },
        count = 3,
    }
end

return multi_arg_process
