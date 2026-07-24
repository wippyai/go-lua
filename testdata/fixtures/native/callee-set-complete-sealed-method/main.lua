-- Contract: a never-mutated local table of methods proves a Complete singleton
-- callee set, but the fact stays revocable by a field store or a metatable
-- install and must publish that deopt-trigger set exhaustively.

local ops = {
    run = function(x: number): number
        return x + 1
    end,
}

local out: number = ops.run(41)

return out
