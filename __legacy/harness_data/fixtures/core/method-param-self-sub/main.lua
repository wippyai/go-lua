type T = { eq: (self: T, other: T) -> boolean }
local t: T = { eq = function(self: T, other: T): boolean return self == other end }
local ok = t:eq(t)
