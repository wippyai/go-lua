local test_state = require("test_state")

local M = {}

function M.patch_state(component_id: string, patch: any): (any, any)
    test_state.state.private_patches[#test_state.state.private_patches + 1] = {
        component_id = component_id,
        patch = patch,
    }
    if test_state.state.patch_state_error then return nil, test_state.state.patch_state_error end
    return { success = true, id = component_id, state = patch }, nil
end

function M.read_public_state(component_id: string): (any, any)
    test_state.state.status_reads[#test_state.state.status_reads + 1] = component_id
    if test_state.state.read_public_error then return nil, test_state.state.read_public_error end
    return test_state.state.public_state or {}, nil
end

function M.read_state(component_id: string): (any, any)
    test_state.state.private_reads[#test_state.state.private_reads + 1] = component_id
    if test_state.state.read_state_error then return nil, test_state.state.read_state_error end
    local row = test_state.state.responders[component_id]
    if type(row) == "table" and type(row.state) == "table" then return row.state, nil end
    return test_state.state.private_state or {}, nil
end

return M

