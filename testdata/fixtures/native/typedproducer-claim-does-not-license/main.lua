-- A cast standing next to a typed producer must not license the producer row.

type Job = {id: string}

local job: Job = {id = "job-1"}
local view = job :: Job
return view.id
