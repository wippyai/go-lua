-- Two construction sites of one record type mint one shape identity; the read
-- site sees a single stable id and an identical field offset from both.

type Point = { x: number, y: number }

local function origin(): Point
    return { x = 0, y = 0 }
end

local function unit(): Point
    return { x = 1, y = 1 }
end

local function sum(p: Point): number
    return p.x + p.y
end

return sum(origin()) + sum(unit())
