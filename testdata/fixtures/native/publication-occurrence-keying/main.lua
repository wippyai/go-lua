-- PUBLICATION IDENTITY: every published row carries its occurrence key -- function
-- generation, executable body, point, site ordinal and source span. A row that
-- cannot be exact-joined to an occurrence or mapped back to source is unusable.
type Point = {
    x: number,
    y: number,
}

local function offset(p: Point, d: number): number
    local base: number = p.x
    local shifted: number = base + d
    return shifted * 2
end

return offset({ x = 1, y = 2 }, 3)
