type Queue = {
    items: { string },
    head: integer,
    set: { [string]: boolean },
}

local function make_queue(): Queue
    return {
        items = {},
        head = 1,
        set = {},
    }
end

local function q_compact(q: Queue): ()
    if q.head < 64 or q.head < (#q.items / 2) then
        return
    end
    local next_items: { string } = {}
    for i = q.head, #q.items do
        local id = q.items[i]
        if q.set[id] then
            table.insert(next_items, id)
        end
    end
    q.items = next_items
    q.head = 1
end

local function q_push(q: Queue, id: string?): ()
    if not id or id == "" or q.set[id] then
        return
    end
    q.set[id] = true
    table.insert(q.items, id)
end

local function q_drop(q: Queue, id: string?): ()
    if not id or id == "" then
        return
    end
    q.set[id] = nil
end

local function q_pop(q: Queue): string?
    while q.head <= #q.items do
        local id = q.items[q.head]
        q.head = q.head + 1
        if q.set[id] then
            q.set[id] = nil
            q_compact(q)
            return id
        end
    end
    q_compact(q)
    return nil
end


local function supervise(pids: { string })
    local live_workers: { [string]: boolean } = {}
    local ready_workers = make_queue()
    local free_claimers = make_queue()

    local function forget_worker(worker_pid: string)
        q_drop(ready_workers, worker_pid)
        live_workers[worker_pid] = nil
    end

    local function forget_claimer(claimer_pid: string, assigned_worker: string?)
        if assigned_worker and live_workers[assigned_worker] then
            q_push(ready_workers, assigned_worker)
        end
        q_drop(free_claimers, claimer_pid)
    end

    local function dispatch_ready(): ()
        while true do
            local worker_pid = q_pop(ready_workers)
            if not worker_pid then
                return
            end
            local claimer_pid = q_pop(free_claimers)
            if not claimer_pid then
                q_push(ready_workers, worker_pid)
                return
            end
        end
    end

    for _, pid in ipairs(pids) do
        live_workers[pid] = true
        q_push(ready_workers, pid)
        q_push(free_claimers, pid .. "-c")
    end
    local rounds = 0
    while rounds < 2 do
        rounds = rounds + 1
        forget_claimer(pids[1] .. "-c", pids[1])
        forget_worker(pids[2])
    end
    q_push(ready_workers, "a")
    q_push(free_claimers, "b-c")
    local worker_pid = q_pop(ready_workers)
    local claimer_pid = q_pop(free_claimers)
    return worker_pid, claimer_pid
end

local w, c = supervise({ "a", "b" })
assert(w == "a" and c == "b-c")
