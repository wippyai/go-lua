-- A captured variable keeps its declared type inside the closures that write
-- and read it (kickside channel hub: bridges: {[string]: Map} filled by one
-- closure, read by another, extended through a local alias).
type Map = { [string]: any }

local function hub()
    local bridges: { [string]: Map } = {}

    local function spawn_for(key: string, pid: string): boolean
        bridges[key] = { pid = pid, session_id = key }
        return true
    end

    local function drop_by_pid(pid: any): any
        for key, b in pairs(bridges) do
            if b.pid == pid then
                local pending = b.pending
                bridges[key] = nil
                return pending
            end
        end
        return nil
    end

    local function hold(key: string, route: any)
        local bridge = bridges[key]
        if bridge then
            bridge.pending = route
        end
    end

    return spawn_for, drop_by_pid, hold
end

return { hub = hub }
