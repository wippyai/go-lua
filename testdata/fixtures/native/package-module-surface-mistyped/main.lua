-- The package manifest declares path as string, loaded as a string-keyed map
-- and loaders as an array. Binding each at a conflicting declared type is
-- refused by the assignment conformance judgment.

local path: number = package.path
local loaded: string = package.loaded
local loaders: string = package.loaders

return path, loaded, loaders
