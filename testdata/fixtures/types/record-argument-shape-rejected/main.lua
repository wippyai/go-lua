type Job = {id: string}

local function accept(out: Job): string
    return out.id
end

local bad = {other = 1}
return accept(bad)
