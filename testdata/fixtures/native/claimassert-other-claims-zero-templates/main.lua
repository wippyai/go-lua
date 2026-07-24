-- A cast and a type annotation are not claim asserts: neither of them demands
-- a throw template.

type Job = {id: string}

local raw: Job = {id = "job-1"}
local job = raw :: Job
local label: string = job.id
return label
