local component = require("component")

local id: string = component.singleton_component_id("component-1")
local wrong_id: number = component.singleton_component_id("component-1")

return id
