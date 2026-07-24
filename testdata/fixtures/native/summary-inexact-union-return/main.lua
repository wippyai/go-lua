-- Contract: a function returning different types on two branches has an
-- over-approximate summary; no call site may elide a result-type guard on it.

local function lookup(flag: boolean): number | string
    if flag then
        return 1
    end
    return "none"
end

return lookup
