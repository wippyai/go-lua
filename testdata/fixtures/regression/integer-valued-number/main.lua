-- From spiralscout.estimation:rank and spiralscout.estimation:read.
local ALPHABET = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
local function char(index: number): string
    return ALPHABET:sub(index + 1, index + 1)
end

local n: any = { depth = 2 }
local indent = string.rep("  ", tonumber(n.depth) or 0)
return char(1) .. indent
