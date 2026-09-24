-- Public surface taken from kickside.automation:stuck_item.
local M = {}
type Map = { [string]: any }
M.REQUESTED_STATE_FOR = { retry = "retry_requested", skip = "skip_requested" }
function M.request(args: any): (Map?, error?)
    return nil, nil
end
function M.list(binding_id: string): ({ Map }?, error?)
    return nil, nil
end
return M
