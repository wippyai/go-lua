-- PUBLICATION IDENTITY: the callee has ONE identity on both sides of the module
-- boundary. The import must not mint a second identity for the same body, or the
-- caller's row and the callee's row cannot be joined.
local helper = require("helper")

local doubled: number = helper.double(21)

return doubled
