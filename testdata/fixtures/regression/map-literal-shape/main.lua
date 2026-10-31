-- From app:durable_observability_tests in spiralscout/market-watch/test.
type Map = { [string]: any }
local OWNER_ONE = { owner_user_ref = "one", workspace_ref = "one-space" }
local OWNER_TWO = { owner_user_ref = "two", workspace_ref = "two-space" }

local function setup(): Map
    local owners: Map = {
        [OWNER_ONE.owner_user_ref] = OWNER_ONE,
        [OWNER_TWO.owner_user_ref] = OWNER_TWO,
    }
    return owners
end

return { setup = setup }
