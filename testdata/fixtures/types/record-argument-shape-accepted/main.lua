type Job = {id: string}

local function accept(out: Job): string
    return out.id
end

local good = {id = "ready"}
return accept(good)
