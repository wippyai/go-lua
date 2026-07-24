-- Parent to child to grandchild: every begin coordinate is published in full
-- edge form rather than as a bare entry position.

local function outer(seed: integer): () -> (() -> integer)
    local base: integer = seed
    return function(): () -> integer
        local step: integer = base + 1
        return function(): integer
            return base + step
        end
    end
end

return outer(1)
