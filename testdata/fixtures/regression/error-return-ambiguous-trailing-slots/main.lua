-- (actor?, scope?, err: string?, kind: string?) has two string slots that could
-- be the error, so no (value, err) correlation holds: err == nil says nothing
-- about the values, which keep their declared types.
local identity = require("identity")

local actor, scope, err, kind = identity.reconstruct({ actor_id = "x", scope_id = "s" })
if err == nil and kind == nil then
    local name = scope.name
    local id = actor:id()
end
