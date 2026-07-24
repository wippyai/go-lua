-- A record annotation drives physical layout, so the producer is runtime
-- relevant and owes its own native operation contract.

type Job = {id: string, retries: integer}

local job: Job = {id = "job-1", retries = 2}
return job.retries
