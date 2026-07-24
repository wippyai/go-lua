-- With an unbounded runtime trip count neither the product accumulator nor the
-- induction variable can be proved overflow-free, so no no-overflow conclusion is
-- published and the per-iteration overflow test stands.

local function product(n: integer): integer
    local sum = 1
    local i = 1
    while i <= n do
        sum = sum * i
        i = i + 1
    end
    return sum
end

return product
