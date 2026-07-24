-- A table literal with a complete static key set and no mutation is closed; the
-- deopt-trigger set is exactly a field store and a metatable install.

local levels = { debug = 10, info = 20, warn = 30, fatal = 40 }

local function severity(verbose: boolean): number
    if verbose then
        return levels.debug
    end
    return levels.warn
end

return severity(false)
