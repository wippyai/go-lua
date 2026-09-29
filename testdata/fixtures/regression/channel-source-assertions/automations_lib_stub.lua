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
return M
