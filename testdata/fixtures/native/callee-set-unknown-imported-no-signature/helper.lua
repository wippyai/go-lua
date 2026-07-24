-- Exports a function value through an any-typed path, so no signature crosses
-- the module boundary with it.

local registry: any = {
    make = function(): number
        return 7
    end,
}

return registry.make
