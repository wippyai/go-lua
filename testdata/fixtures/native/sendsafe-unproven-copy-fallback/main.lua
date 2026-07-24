-- SEND SAFETY: the payload is a named local built up over several statements and
-- still read by the sender afterwards, so its alias set is not closed. The
-- verdict is the explicit copy fallback, published as a row rather than omitted.
type Batch = {
    id: string,
    size: number,
}

local pid: string = "collector"

local batch: Batch = { id = "b-1", size = 0 }
batch.size = 2

process.send(pid, "batch", batch)

local retained: string = batch.id

return retained
