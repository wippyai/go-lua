-- Bee runtime-pin checker regression: bee.desktop:model:350
-- Expected vs actual: Valid nested updates preserve Window[]; actual argument expected Window[], got partial-record union array.
-- Verified: go-lua v1.5.24 passes (0 diagnostics); v1.6.0 fails.
-- Run with pinlint-driver/v1524/checker or pinlint-driver/v160/checker.
type Rect = {x: integer, y: integer}
type Window = {normal_bounds: Rect, bounds: Rect, mode: "collapsed" | "floating"}
type Scene = {windows: {Window}}
local function copy_windows(values: {Window}): {Window}
    local result: {Window} = {}
    for i = 1, #values do
        result[i] = {normal_bounds = {x = values[i].normal_bounds.x, y = values[i].normal_bounds.y}, bounds = values[i].bounds, mode = values[i].mode}
    end
    return result
end
local function commit(windows: {Window}): {Window} return windows end
local function place(scene: Scene, index: integer, placed: Rect): {Window}
    local current = scene.windows[index]
    if current.mode == "collapsed" then
        local windows = copy_windows(scene.windows)
        windows[index].bounds = placed
        windows[index].normal_bounds.x = placed.x
        windows[index].normal_bounds.y = placed.y
        return commit(windows)
    end
    return scene.windows
end
return place
