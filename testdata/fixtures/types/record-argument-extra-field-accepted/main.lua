type Job = {id: string}

local function accept(out: Job): string
    return out.id
end

local wide = {id = "ready", other = 1}
return accept(wide)
