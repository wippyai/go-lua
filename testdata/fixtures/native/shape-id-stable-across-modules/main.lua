-- Structural shape identity is one value across the module boundary: the reader
-- keys its inline cache on the id minted by the producing module.

local shapes = require("shapes")

local p = shapes.origin()
return p.x + p.y
