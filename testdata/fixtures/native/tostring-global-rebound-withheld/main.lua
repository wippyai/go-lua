-- The module rebinds tostring before the call site, so the name no longer
-- denotes the builtin and no builtin row may be published for the call.

local function shout(v: any): string
    return "!"
end

tostring = shout

local label: string = tostring(41)
return label
