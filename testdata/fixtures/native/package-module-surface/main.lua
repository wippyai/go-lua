-- The stdlib package module declares two functions and six values. Each member
-- is bound here at the type its manifest declaration states, so the whole
-- declared surface is exercised on the arm that carries no refutation.

local path: string = package.path
local cpath: string = package.cpath
local config: string = package.config

local loaded_entry: any = package.loaded["string"]
local preload_entry: any = package.preload["mymod"]
local loader: any = package.loaders[1]

local loadlib = package.loadlib

package.seeall({})

return path, cpath, config, loaded_entry, preload_entry, loader, loadlib
